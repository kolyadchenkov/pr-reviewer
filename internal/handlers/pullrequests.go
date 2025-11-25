package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"pr-reviewer-service/internal/models"
)

func (h *Handler) HandlePRCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}

	var payload models.PRCreatePayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid json")
		return
	}

	payload.ID = strings.TrimSpace(payload.ID)
	payload.Name = strings.TrimSpace(payload.Name)
	payload.AuthorID = strings.TrimSpace(payload.AuthorID)

	if payload.ID == "" || payload.Name == "" || payload.AuthorID == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "all fields are required")
		return
	}

	ctx := getContext(r)

	exists, err := h.repo.PRExists(ctx, payload.ID)
	if err != nil {
		writeInternal(w)
		return
	}
	if exists {
		writeError(w, http.StatusConflict, "PR_EXISTS", "PR id already exists")
		return
	}

	teamName, err := h.repo.GetUserTeam(ctx, payload.AuthorID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "author not found")
			return
		}
		writeInternal(w)
		return
	}

	if err := h.repo.CreatePR(ctx, &payload); err != nil {
		writeInternal(w)
		return
	}

	candidates, err := h.repo.GetCandidates(ctx, teamName, []string{payload.AuthorID}, 2)
	if err != nil {
		writeInternal(w)
		return
	}

	if len(candidates) > 0 {
		if err := h.repo.AssignReviewers(ctx, payload.ID, candidates); err != nil {
			writeInternal(w)
			return
		}
	}

	pr, err := h.repo.GetPR(ctx, payload.ID)
	if err != nil {
		writeInternal(w)
		return
	}

	writeJSON(w, http.StatusCreated, models.PRResponse{PR: *pr})
}

func (h *Handler) HandlePRMerge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}

	var payload models.PRMergePayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid json")
		return
	}

	payload.ID = strings.TrimSpace(payload.ID)
	if payload.ID == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "pull_request_id is required")
		return
	}

	ctx := getContext(r)

	_, err := h.repo.GetPR(ctx, payload.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "PR not found")
			return
		}
		writeInternal(w)
		return
	}

	if err := h.repo.MergePR(ctx, payload.ID); err != nil {
		writeInternal(w)
		return
	}

	pr, err := h.repo.GetPR(ctx, payload.ID)
	if err != nil {
		writeInternal(w)
		return
	}

	writeJSON(w, http.StatusOK, models.PRResponse{PR: *pr})
}

func (h *Handler) HandlePRReassign(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}

	var payload models.PRReassignPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid json")
		return
	}
	payload.ID = strings.TrimSpace(payload.ID)
	payload.OldUserID = strings.TrimSpace(payload.OldUserID)
	if payload.ID == "" || payload.OldUserID == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "pull_request_id and old_user_id required")
		return
	}

	ctx := getContext(r)

	status, err := h.repo.GetPRStatus(ctx, payload.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "PR not found")
			return
		}
		writeInternal(w)
		return
	}

	if status == models.StatusMerged {
		writeError(w, http.StatusConflict, "PR_MERGED", "cannot reassign on merged PR")
		return
	}

	assigned, err := h.repo.IsReviewerAssigned(ctx, payload.ID, payload.OldUserID)
	if err != nil {
		writeInternal(w)
		return
	}
	if !assigned {
		writeError(w, http.StatusConflict, "NOT_ASSIGNED", "reviewer is not assigned to this PR")
		return
	}

	teamName, err := h.repo.GetUserTeam(ctx, payload.OldUserID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "user not found")
			return
		}
		writeInternal(w)
		return
	}

	authorID, err := h.repo.GetPRAuthor(ctx, payload.ID)
	if err != nil {
		writeInternal(w)
		return
	}

	replacement, err := h.repo.GetReplacementCandidate(ctx, teamName, []string{payload.OldUserID, authorID}, payload.ID)
	if err != nil {
		if err.Error() == "no candidate found" {
			writeError(w, http.StatusConflict, "NO_CANDIDATE", "no active replacement candidate in team")
			return
		}
		writeInternal(w)
		return
	}

	if err := h.repo.ReassignReviewer(ctx, payload.ID, payload.OldUserID, replacement); err != nil {
		writeInternal(w)
		return
	}

	pr, err := h.repo.GetPR(ctx, payload.ID)
	if err != nil {
		writeInternal(w)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"pr":          *pr,
		"replaced_by": replacement,
	})
}
