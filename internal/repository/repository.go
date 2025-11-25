package repository

import (
	"context"
	"pr-reviewer-service/internal/models"
)

type Repository interface {
	// teams
	TeamExists(ctx context.Context, teamName string) (bool, error)
	CreateTeam(ctx context.Context, team *models.TeamPayload) error
	GetTeam(ctx context.Context, teamName string) (*models.TeamPayload, error)

	// users
	UserExists(ctx context.Context, userID string) (bool, error)
	GetUserTeam(ctx context.Context, userID string) (string, error)
	UpdateUserActive(ctx context.Context, userID string, isActive bool) (*models.User, error)
	GetUserPRs(ctx context.Context, userID string) ([]models.PullRequestShort, error)

	// pull requests
	PRExists(ctx context.Context, prID string) (bool, error)
	GetPR(ctx context.Context, prID string) (*models.PullRequest, error)
	GetPRStatus(ctx context.Context, prID string) (string, error)
	GetPRAuthor(ctx context.Context, prID string) (string, error)
	CreatePR(ctx context.Context, pr *models.PRCreatePayload) error
	GetCandidates(ctx context.Context, teamName string, excludeUserIDs []string, limit int) ([]string, error)
	AssignReviewers(ctx context.Context, prID string, reviewerIDs []string) error
	MergePR(ctx context.Context, prID string) error
	IsReviewerAssigned(ctx context.Context, prID string, userID string) (bool, error)
	GetReplacementCandidate(ctx context.Context, teamName string, excludeUserIDs []string, prID string) (string, error)
	ReassignReviewer(ctx context.Context, prID string, oldUserID string, newUserID string) error

	// stats
	GetStats(ctx context.Context) (*models.StatsResponse, error)
}
