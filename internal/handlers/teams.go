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

	for _, m := range payload.Members {
		if strings.TrimSpace(m.UserID) == "" {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "user_id is required")
			return
		}
		if strings.TrimSpace(m.Username) == "" {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "username is required")
			return
		}
	}

	ctx := getContext(r)

	exists, err := h.repo.TeamExists(ctx, payload.TeamName)
	if err != nil {
		writeInternal(w)
		return
	}
	if exists {
		writeError(w, http.StatusBadRequest, "TEAM_EXISTS", "team_name already exists")
		return
	}

	if err := h.repo.CreateTeam(ctx, &payload); err != nil {
		writeInternal(w)
		return
	}

	team, err := h.repo.GetTeam(ctx, payload.TeamName)
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

	ctx := getContext(r)
	team, err := h.repo.GetTeam(ctx, name)
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
