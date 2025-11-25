package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"pr-reviewer-service/internal/models"
	"pr-reviewer-service/internal/repository"

	"github.com/lib/pq"
)

var _ repository.Repository = (*PostgresRepository)(nil)

type PostgresRepository struct {
	db *sql.DB
}

func New(db *sql.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

// teams

func (r *PostgresRepository) TeamExists(ctx context.Context, teamName string) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM teams WHERE team_name=$1)", teamName).Scan(&exists)
	return exists, err
}

func (r *PostgresRepository) CreateTeam(ctx context.Context, team *models.TeamPayload) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, "INSERT INTO teams(team_name) VALUES($1)", team.TeamName); err != nil {
		return err
	}

	for _, m := range team.Members {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO users(user_id, username, team_name, is_active)
			VALUES($1, $2, $3, $4)
			ON CONFLICT (user_id) DO UPDATE SET
				username=EXCLUDED.username,
				team_name=EXCLUDED.team_name,
				is_active=EXCLUDED.is_active
		`, m.UserID, m.Username, team.TeamName, m.IsActive)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (r *PostgresRepository) GetTeam(ctx context.Context, teamName string) (*models.TeamPayload, error) {
	var result models.TeamPayload
	row := r.db.QueryRowContext(ctx, "SELECT team_name FROM teams WHERE team_name=$1", teamName)
	if err := row.Scan(&result.TeamName); err != nil {
		return nil, err
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT user_id, username, is_active FROM users WHERE team_name=$1 ORDER BY user_id
	`, teamName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var m models.TeamMember
		if err := rows.Scan(&m.UserID, &m.Username, &m.IsActive); err != nil {
			return nil, err
		}
		result.Members = append(result.Members, m)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return &result, nil
}

// users

func (r *PostgresRepository) UserExists(ctx context.Context, userID string) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM users WHERE user_id=$1)", userID).Scan(&exists)
	return exists, err
}

func (r *PostgresRepository) GetUserTeam(ctx context.Context, userID string) (string, error) {
	var teamName string
	err := r.db.QueryRowContext(ctx, "SELECT team_name FROM users WHERE user_id=$1", userID).Scan(&teamName)
	if err != nil {
		return "", err
	}
	return teamName, nil
}

func (r *PostgresRepository) UpdateUserActive(ctx context.Context, userID string, isActive bool) (*models.User, error) {
	var usr models.User
	err := r.db.QueryRowContext(ctx, `
		UPDATE users SET is_active=$1
		WHERE user_id=$2
		RETURNING user_id, username, team_name, is_active
	`, isActive, userID).Scan(&usr.UserID, &usr.Username, &usr.TeamName, &usr.IsActive)
	if err != nil {
		return nil, err
	}
	return &usr, nil
}

func (r *PostgresRepository) GetUserPRs(ctx context.Context, userID string) ([]models.PullRequestShort, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT pr.pull_request_id, pr.pull_request_name, pr.author_id, pr.status
		FROM pull_requests pr
		INNER JOIN pull_request_reviewers rr ON rr.pr_id = pr.pull_request_id
		WHERE rr.reviewer_id=$1
		ORDER BY pr.created_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var prs []models.PullRequestShort
	for rows.Next() {
		var item models.PullRequestShort
		if err := rows.Scan(&item.ID, &item.Name, &item.AuthorID, &item.Status); err != nil {
			return nil, err
		}
		prs = append(prs, item)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return prs, nil
}

// pull requests

func (r *PostgresRepository) PRExists(ctx context.Context, prID string) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM pull_requests WHERE pull_request_id=$1)", prID).Scan(&exists)
	return exists, err
}

func (r *PostgresRepository) GetPR(ctx context.Context, prID string) (*models.PullRequest, error) {
	var pr models.PullRequest
	var createdAt time.Time
	var mergedAt sql.NullTime

	err := r.db.QueryRowContext(ctx, `
		SELECT pull_request_id, pull_request_name, author_id, status, created_at, merged_at
		FROM pull_requests
		WHERE pull_request_id=$1
	`, prID).Scan(&pr.ID, &pr.Name, &pr.AuthorID, &pr.Status, &createdAt, &mergedAt)
	if err != nil {
		return nil, err
	}

	pr.CreatedAt = &createdAt
	if mergedAt.Valid {
		pr.MergedAt = &mergedAt.Time
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT reviewer_id FROM pull_request_reviewers WHERE pr_id=$1 ORDER BY reviewer_id
	`, prID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var reviewer string
		if err := rows.Scan(&reviewer); err != nil {
			return nil, err
		}
		pr.Reviewers = append(pr.Reviewers, reviewer)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return &pr, nil
}

func (r *PostgresRepository) GetPRStatus(ctx context.Context, prID string) (string, error) {
	var status string
	err := r.db.QueryRowContext(ctx, "SELECT status FROM pull_requests WHERE pull_request_id=$1", prID).Scan(&status)
	if err != nil {
		return "", err
	}
	return status, nil
}

func (r *PostgresRepository) GetPRAuthor(ctx context.Context, prID string) (string, error) {
	var authorID string
	err := r.db.QueryRowContext(ctx, "SELECT author_id FROM pull_requests WHERE pull_request_id=$1", prID).Scan(&authorID)
	if err != nil {
		return "", err
	}
	return authorID, nil
}

func (r *PostgresRepository) CreatePR(ctx context.Context, pr *models.PRCreatePayload) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO pull_requests(pull_request_id, pull_request_name, author_id, status)
		VALUES($1, $2, $3, $4)
	`, pr.ID, pr.Name, pr.AuthorID, models.StatusOpen)
	return err
}

func (r *PostgresRepository) GetCandidates(ctx context.Context, teamName string, excludeUserIDs []string, limit int) ([]string, error) {
	query := `
		SELECT user_id FROM users
		WHERE team_name=$1 AND is_active=true
	`
	args := []interface{}{teamName}
	argPos := 2

	if len(excludeUserIDs) > 0 {
		query += " AND user_id != ALL($" + string(rune('0'+argPos)) + "::text[])"
		args = append(args, pq.Array(excludeUserIDs))
		argPos++
	}

	query += " ORDER BY random() LIMIT $" + string(rune('0'+argPos))
	args = append(args, limit)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var candidates []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		candidates = append(candidates, id)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return candidates, nil
}

func (r *PostgresRepository) AssignReviewers(ctx context.Context, prID string, reviewerIDs []string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, reviewerID := range reviewerIDs {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO pull_request_reviewers(pr_id, reviewer_id)
			VALUES($1, $2)
		`, prID, reviewerID); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (r *PostgresRepository) MergePR(ctx context.Context, prID string) error {
	now := time.Now().UTC()
	_, err := r.db.ExecContext(ctx, `
		UPDATE pull_requests SET status=$1, merged_at=$2 WHERE pull_request_id=$3 AND status!=$1
	`, models.StatusMerged, now, prID)
	return err
}

func (r *PostgresRepository) IsReviewerAssigned(ctx context.Context, prID string, userID string) (bool, error) {
	var assigned bool
	err := r.db.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM pull_request_reviewers WHERE pr_id=$1 AND reviewer_id=$2
		)
	`, prID, userID).Scan(&assigned)
	return assigned, err
}

func (r *PostgresRepository) GetReplacementCandidate(ctx context.Context, teamName string, excludeUserIDs []string, prID string) (string, error) {
	query := `
		SELECT user_id FROM users
		WHERE team_name=$1 AND is_active=true
	`
	args := []interface{}{teamName}
	argPos := 2

	if len(excludeUserIDs) > 0 {
		query += " AND user_id != ALL($" + string(rune('0'+argPos)) + "::text[])"
		args = append(args, pq.Array(excludeUserIDs))
		argPos++
	}

	query += " AND user_id NOT IN (SELECT reviewer_id FROM pull_request_reviewers WHERE pr_id=$" + string(rune('0'+argPos)) + ")"
	args = append(args, prID)
	argPos++

	query += " ORDER BY random() LIMIT 1"

	var replacement string
	err := r.db.QueryRowContext(ctx, query, args...).Scan(&replacement)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", errors.New("no candidate found")
		}
		return "", err
	}
	return replacement, nil
}

func (r *PostgresRepository) ReassignReviewer(ctx context.Context, prID string, oldUserID string, newUserID string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
		DELETE FROM pull_request_reviewers WHERE pr_id=$1 AND reviewer_id=$2
	`, prID, oldUserID); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO pull_request_reviewers(pr_id, reviewer_id) VALUES($1, $2)
	`, prID, newUserID); err != nil {
		return err
	}

	return tx.Commit()
}

// stats

func (r *PostgresRepository) GetStats(ctx context.Context) (*models.StatsResponse, error) {
	resp := &models.StatsResponse{}

	rowsReviewers, err := r.db.QueryContext(ctx, `
		SELECT u.user_id, u.username, COUNT(rr.pr_id) AS total
		FROM users u
		LEFT JOIN pull_request_reviewers rr ON rr.reviewer_id = u.user_id
		GROUP BY u.user_id, u.username
		HAVING COUNT(rr.pr_id) > 0
		ORDER BY total DESC, u.user_id
	`)
	if err != nil {
		return nil, err
	}
	defer rowsReviewers.Close()

	for rowsReviewers.Next() {
		var item models.ReviewerStat
		if err := rowsReviewers.Scan(&item.UserID, &item.Username, &item.AssignedPRs); err != nil {
			return nil, err
		}
		resp.Reviewers = append(resp.Reviewers, item)
	}

	if err := rowsReviewers.Err(); err != nil {
		return nil, err
	}

	rowsPR, err := r.db.QueryContext(ctx, `
		SELECT pr.pull_request_id, COUNT(rr.reviewer_id) AS total
		FROM pull_requests pr
		LEFT JOIN pull_request_reviewers rr ON rr.pr_id = pr.pull_request_id
		GROUP BY pr.pull_request_id
		HAVING COUNT(rr.reviewer_id) > 0
		ORDER BY pr.pull_request_id
	`)
	if err != nil {
		return nil, err
	}
	defer rowsPR.Close()

	for rowsPR.Next() {
		var item models.PRStat
		if err := rowsPR.Scan(&item.PullRequestID, &item.Reviewers); err != nil {
			return nil, err
		}
		resp.PullReqs = append(resp.PullReqs, item)
	}

	if err := rowsPR.Err(); err != nil {
		return nil, err
	}

	return resp, nil
}
