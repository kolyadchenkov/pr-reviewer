package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

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

	tx, err := h.db.Begin()
	if err != nil {
		writeInternal(w)
		return
	}
	defer tx.Rollback()

	var exists bool
	if err := tx.QueryRow("SELECT EXISTS(SELECT 1 FROM pull_requests WHERE pull_request_id=$1)", payload.ID).Scan(&exists); err != nil {
		writeInternal(w)
		return
	}
	if exists {
		writeError(w, http.StatusConflict, "PR_EXISTS", "PR id already exists")
		return
	}

	var teamName string
	if err := tx.QueryRow("SELECT team_name FROM users WHERE user_id=$1", payload.AuthorID).Scan(&teamName); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "author not found")
			return
		}
		writeInternal(w)
		return
	}

	_, err = tx.Exec(`
		INSERT INTO pull_requests(pull_request_id, pull_request_name, author_id, status)
		VALUES($1, $2, $3, $4)
	`, payload.ID, payload.Name, payload.AuthorID, models.StatusOpen)
	if err != nil {
		writeInternal(w)
		return
	}

	candidatesRows, err := tx.Query(`
		SELECT user_id FROM users
		WHERE team_name=$1 AND is_active=true AND user_id<>$2
		ORDER BY random()
		LIMIT 2
	`, teamName, payload.AuthorID)
	if err != nil {
		writeInternal(w)
		return
	}
	defer candidatesRows.Close()

	var reviewers []string
	for candidatesRows.Next() {
		var id string
		if err := candidatesRows.Scan(&id); err != nil {
			writeInternal(w)
			return
		}
		reviewers = append(reviewers, id)
	}
	if err := candidatesRows.Err(); err != nil {
		writeInternal(w)
		return
	}

	for _, reviewer := range reviewers {
		if _, err := tx.Exec(`
			INSERT INTO pull_request_reviewers(pr_id, reviewer_id)
			VALUES($1, $2)
		`, payload.ID, reviewer); err != nil {
			writeInternal(w)
			return
		}
	}

	if err := tx.Commit(); err != nil {
		writeInternal(w)
		return
	}

	pr, err := h.getPR(payload.ID)
	if err != nil {
		writeInternal(w)
		return
	}

	writeJSON(w, http.StatusCreated, models.PRResponse{PR: pr})
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

	tx, err := h.db.Begin()
	if err != nil {
		writeInternal(w)
		return
	}
	defer tx.Rollback()

	var status string
	var mergedAt sql.NullTime
	if err := tx.QueryRow(`
		SELECT status, merged_at FROM pull_requests WHERE pull_request_id=$1
	`, payload.ID).Scan(&status, &mergedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "PR not found")
			return
		}
		writeInternal(w)
		return
	}

	now := time.Now().UTC()
	if status != models.StatusMerged {
		if _, err := tx.Exec(`
			UPDATE pull_requests SET status=$1, merged_at=$2 WHERE pull_request_id=$3
		`, models.StatusMerged, now, payload.ID); err != nil {
			writeInternal(w)
			return
		}
		if err := tx.Commit(); err != nil {
			writeInternal(w)
			return
		}
	} else {
		if err := tx.Commit(); err != nil {
			writeInternal(w)
			return
		}
	}

	pr, err := h.getPR(payload.ID)
	if err != nil {
		writeInternal(w)
		return
	}
	writeJSON(w, http.StatusOK, models.PRResponse{PR: pr})
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

	tx, err := h.db.Begin()
	if err != nil {
		writeInternal(w)
		return
	}
	defer tx.Rollback()

	var status, authorID string
	if err := tx.QueryRow(`
		SELECT status, author_id FROM pull_requests WHERE pull_request_id=$1
	`, payload.ID).Scan(&status, &authorID); err != nil {
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

	var assigned bool
	if err := tx.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM pull_request_reviewers WHERE pr_id=$1 AND reviewer_id=$2
		)
	`, payload.ID, payload.OldUserID).Scan(&assigned); err != nil {
		writeInternal(w)
		return
	}
	if !assigned {
		writeError(w, http.StatusConflict, "NOT_ASSIGNED", "reviewer is not assigned to this PR")
		return
	}

	var teamName string
	if err := tx.QueryRow(`
		SELECT team_name FROM users WHERE user_id=$1
	`, payload.OldUserID).Scan(&teamName); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "user not found")
			return
		}
		writeInternal(w)
		return
	}

	row := tx.QueryRow(`
		SELECT user_id FROM users
		WHERE team_name=$1 AND is_active=true AND user_id<>$2 AND user_id<>$3
		AND user_id NOT IN (SELECT reviewer_id FROM pull_request_reviewers WHERE pr_id=$4)
		ORDER BY random()
		LIMIT 1
	`, teamName, payload.OldUserID, authorID, payload.ID)

	var replacement string
	if err := row.Scan(&replacement); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusConflict, "NO_CANDIDATE", "no active replacement candidate in team")
			return
		}
		writeInternal(w)
		return
	}

	if _, err := tx.Exec(`
		DELETE FROM pull_request_reviewers WHERE pr_id=$1 AND reviewer_id=$2
	`, payload.ID, payload.OldUserID); err != nil {
		writeInternal(w)
		return
	}

	if _, err := tx.Exec(`
		INSERT INTO pull_request_reviewers(pr_id, reviewer_id) VALUES($1, $2)
	`, payload.ID, replacement); err != nil {
		writeInternal(w)
		return
	}

	if err := tx.Commit(); err != nil {
		writeInternal(w)
		return
	}

	pr, err := h.getPR(payload.ID)
	if err != nil {
		writeInternal(w)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"pr":          pr,
		"replaced_by": replacement,
	})
}

func (h *Handler) getPR(id string) (models.PullRequest, error) {
	var pr models.PullRequest
	var createdAt time.Time
	var mergedAt sql.NullTime

	err := h.db.QueryRow(`
		SELECT pull_request_id, pull_request_name, author_id, status, created_at, merged_at
		FROM pull_requests
		WHERE pull_request_id=$1
	`, id).Scan(&pr.ID, &pr.Name, &pr.AuthorID, &pr.Status, &createdAt, &mergedAt)
	if err != nil {
		return pr, err
	}

	pr.CreatedAt = &createdAt
	if mergedAt.Valid {
		pr.MergedAt = &mergedAt.Time
	}

	rows, err := h.db.Query(`
		SELECT reviewer_id FROM pull_request_reviewers WHERE pr_id=$1 ORDER BY reviewer_id
	`, id)
	if err != nil {
		return pr, err
	}
	defer rows.Close()

	for rows.Next() {
		var reviewer string
		if err := rows.Scan(&reviewer); err != nil {
			return pr, err
		}
		pr.Reviewers = append(pr.Reviewers, reviewer)
	}
	if err := rows.Err(); err != nil {
		return pr, err
	}

	return pr, nil
}

