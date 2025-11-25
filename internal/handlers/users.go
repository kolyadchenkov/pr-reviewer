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

	ctx := getContext(r)
	usr, err := h.repo.UpdateUserActive(ctx, payload.UserID, payload.IsActive)
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

	ctx := getContext(r)
	exists, err := h.repo.UserExists(ctx, userID)
	if err != nil {
		writeInternal(w)
		return
	}
	if !exists {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "user not found")
		return
	}

	prs, err := h.repo.GetUserPRs(ctx, userID)
	if err != nil {
		writeInternal(w)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"user_id":       userID,
		"pull_requests": prs,
	})
}

