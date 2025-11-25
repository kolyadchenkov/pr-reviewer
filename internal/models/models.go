package models

import "time"

const (
	StatusOpen   = "OPEN"
	StatusMerged = "MERGED"
)

type ErrorResponse struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

type TeamMember struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	IsActive bool   `json:"is_active"`
}

type TeamPayload struct {
	TeamName string       `json:"team_name"`
	Members  []TeamMember `json:"members"`
}

type User struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	TeamName string `json:"team_name"`
	IsActive bool   `json:"is_active"`
}

type PullRequest struct {
	ID        string     `json:"pull_request_id"`
	Name      string     `json:"pull_request_name"`
	AuthorID  string     `json:"author_id"`
	Status    string     `json:"status"`
	Reviewers []string   `json:"assigned_reviewers"`
	CreatedAt *time.Time `json:"createdAt"`
	MergedAt  *time.Time `json:"mergedAt"`
}

type PullRequestShort struct {
	ID       string `json:"pull_request_id"`
	Name     string `json:"pull_request_name"`
	AuthorID string `json:"author_id"`
	Status   string `json:"status"`
}

type PRCreatePayload struct {
	ID       string `json:"pull_request_id"`
	Name     string `json:"pull_request_name"`
	AuthorID string `json:"author_id"`
}

type PRMergePayload struct {
	ID string `json:"pull_request_id"`
}

type PRReassignPayload struct {
	ID        string `json:"pull_request_id"`
	OldUserID string `json:"old_user_id"`
}

type UserActivePayload struct {
	UserID   string `json:"user_id"`
	IsActive bool   `json:"is_active"`
}

type PRResponse struct {
	PR PullRequest `json:"pr"`
}

type StatsResponse struct {
	Reviewers []ReviewerStat `json:"reviewers"`
	PullReqs  []PRStat       `json:"pull_requests"`
}

type ReviewerStat struct {
	UserID      string `json:"user_id"`
	Username    string `json:"username"`
	AssignedPRs int    `json:"assigned_prs"`
}

type PRStat struct {
	PullRequestID string `json:"pull_request_id"`
	Reviewers     int    `json:"reviewers"`
}

