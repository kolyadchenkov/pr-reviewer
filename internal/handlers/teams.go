package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"pr-reviewer-service/internal/models"
)

func (h *Handler) HandleTeamAdd(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}

	var payload models.TeamPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid json")
		return
	}

	payload.TeamName = strings.TrimSpace(payload.TeamName)
	if payload.TeamName == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "team_name is required")
		return
	}

	tx, err := h.db.Begin()
	if err != nil {
		writeInternal(w)
		return
	}
	defer tx.Rollback()

	var exists bool
	if err := tx.QueryRow("SELECT EXISTS(SELECT 1 FROM teams WHERE team_name=$1)", payload.TeamName).Scan(&exists); err != nil {
		writeInternal(w)
		return
	}
	if exists {
		writeError(w, http.StatusBadRequest, "TEAM_EXISTS", "team_name already exists")
		return
	}

	if _, err := tx.Exec("INSERT INTO teams(team_name) VALUES($1)", payload.TeamName); err != nil {
		writeInternal(w)
		return
	}

	for _, m := range payload.Members {
		if strings.TrimSpace(m.UserID) == "" {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "user_id is required")
			return
		}
		if strings.TrimSpace(m.Username) == "" {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "username is required")
			return
		}

		_, err := tx.Exec(`
			INSERT INTO users(user_id, username, team_name, is_active)
			VALUES($1, $2, $3, $4)
			ON CONFLICT (user_id) DO UPDATE SET
				username=EXCLUDED.username,
				team_name=EXCLUDED.team_name,
				is_active=EXCLUDED.is_active
		`, m.UserID, m.Username, payload.TeamName, m.IsActive)
		if err != nil {
			writeInternal(w)
			return
		}
	}

	if err := tx.Commit(); err != nil {
		writeInternal(w)
		return
	}

	team, err := h.getTeam(payload.TeamName)
	if err != nil {
		writeInternal(w)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{"team": team})
}

func (h *Handler) HandleTeamGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}

	name := strings.TrimSpace(r.URL.Query().Get("team_name"))
	if name == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "team_name is required")
		return
	}

	team, err := h.getTeam(name)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "team not found")
			return
		}
		writeInternal(w)
		return
	}

	writeJSON(w, http.StatusOK, team)
}

func (h *Handler) getTeam(name string) (models.TeamPayload, error) {
	var result models.TeamPayload
	row := h.db.QueryRow("SELECT team_name FROM teams WHERE team_name=$1", name)
	if err := row.Scan(&result.TeamName); err != nil {
		return result, err
	}

	rows, err := h.db.Query(`
		SELECT user_id, username, is_active FROM users WHERE team_name=$1 ORDER BY user_id
	`, name)
	if err != nil {
		return result, err
	}
	defer rows.Close()

	for rows.Next() {
		var m models.TeamMember
		if err := rows.Scan(&m.UserID, &m.Username, &m.IsActive); err != nil {
			return result, err
		}
		result.Members = append(result.Members, m)
	}

	return result, rows.Err()
}

