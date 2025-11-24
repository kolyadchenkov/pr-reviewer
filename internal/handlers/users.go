package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"pr-reviewer-service/internal/models"
)

func (h *Handler) HandleUserSetActive(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}

	var payload models.UserActivePayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid json")
		return
	}

	if strings.TrimSpace(payload.UserID) == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "user_id is required")
		return
	}

	var usr models.User
	err := h.db.QueryRow(`
		UPDATE users SET is_active=$1
		WHERE user_id=$2
		RETURNING user_id, username, team_name, is_active
	`, payload.IsActive, payload.UserID).Scan(&usr.UserID, &usr.Username, &usr.TeamName, &usr.IsActive)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "user not found")
			return
		}
		writeInternal(w)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"user": usr})
}

func (h *Handler) HandleUserReviews(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}

	userID := strings.TrimSpace(r.URL.Query().Get("user_id"))
	if userID == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "user_id is required")
		return
	}

	var exists bool
	if err := h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM users WHERE user_id=$1)", userID).Scan(&exists); err != nil {
		writeInternal(w)
		return
	}
	if !exists {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "user not found")
		return
	}

	rows, err := h.db.Query(`
		SELECT pr.pull_request_id, pr.pull_request_name, pr.author_id, pr.status
		FROM pull_requests pr
		INNER JOIN pull_request_reviewers rr ON rr.pr_id = pr.pull_request_id
		WHERE rr.reviewer_id=$1
		ORDER BY pr.created_at DESC
	`, userID)
	if err != nil {
		writeInternal(w)
		return
	}
	defer rows.Close()

	var prs []models.PullRequestShort
	for rows.Next() {
		var item models.PullRequestShort
		if err := rows.Scan(&item.ID, &item.Name, &item.AuthorID, &item.Status); err != nil {
			writeInternal(w)
			return
		}
		prs = append(prs, item)
	}
	if err := rows.Err(); err != nil {
		writeInternal(w)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"user_id":       userID,
		"pull_requests": prs,
	})
}

