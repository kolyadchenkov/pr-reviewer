package handlers

import (
	"net/http"

	"pr-reviewer-service/internal/models"
)

func (h *Handler) HandleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}

	resp := models.StatsResponse{}

	rowsReviewers, err := h.db.Query(`
		SELECT u.user_id, u.username, COUNT(rr.pr_id) AS total
		FROM users u
		LEFT JOIN pull_request_reviewers rr ON rr.reviewer_id = u.user_id
		GROUP BY u.user_id, u.username
		HAVING COUNT(rr.pr_id) > 0
		ORDER BY total DESC, u.user_id
	`)
	if err != nil {
		writeInternal(w)
		return
	}
	defer rowsReviewers.Close()

	for rowsReviewers.Next() {
		var item models.ReviewerStat
		if err := rowsReviewers.Scan(&item.UserID, &item.Username, &item.AssignedPRs); err != nil {
			writeInternal(w)
			return
		}
		resp.Reviewers = append(resp.Reviewers, item)
	}
	if err := rowsReviewers.Err(); err != nil {
		writeInternal(w)
		return
	}

	rowsPR, err := h.db.Query(`
		SELECT pr.pull_request_id, COUNT(rr.reviewer_id) AS total
		FROM pull_requests pr
		LEFT JOIN pull_request_reviewers rr ON rr.pr_id = pr.pull_request_id
		GROUP BY pr.pull_request_id
		HAVING COUNT(rr.reviewer_id) > 0
		ORDER BY pr.pull_request_id
	`)
	if err != nil {
		writeInternal(w)
		return
	}
	defer rowsPR.Close()

	for rowsPR.Next() {
		var item models.PRStat
		if err := rowsPR.Scan(&item.PullRequestID, &item.Reviewers); err != nil {
			writeInternal(w)
			return
		}
		resp.PullReqs = append(resp.PullReqs, item)
	}
	if err := rowsPR.Err(); err != nil {
		writeInternal(w)
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

