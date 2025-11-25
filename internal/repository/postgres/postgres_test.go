package postgres

import (
	"context"
	"database/sql"
	"testing"

	"pr-reviewer-service/internal/models"

	_ "github.com/lib/pq"
)

func setupTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("postgres", "postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable")
	if err != nil {
		t.Fatalf("Failed to connect to test database: %v", err)
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS teams (
			team_name TEXT PRIMARY KEY,
			created_at TIMESTAMPTZ DEFAULT NOW()
		);
		
		CREATE TABLE IF NOT EXISTS users (
			user_id TEXT PRIMARY KEY,
			username TEXT NOT NULL,
			team_name TEXT NOT NULL REFERENCES teams(team_name) ON DELETE CASCADE,
			is_active BOOLEAN NOT NULL
		);
		
		CREATE TABLE IF NOT EXISTS pull_requests (
			pull_request_id TEXT PRIMARY KEY,
			pull_request_name TEXT NOT NULL,
			author_id TEXT NOT NULL REFERENCES users(user_id),
			status TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			merged_at TIMESTAMPTZ
		);
		
		CREATE TABLE IF NOT EXISTS pull_request_reviewers (
			pr_id TEXT NOT NULL REFERENCES pull_requests(pull_request_id) ON DELETE CASCADE,
			reviewer_id TEXT NOT NULL REFERENCES users(user_id),
			PRIMARY KEY(pr_id, reviewer_id)
		);
	`)
	if err != nil {
		t.Fatalf("Failed to create test tables: %v", err)
	}

	return db
}

func cleanupTestDB(t *testing.T, db *sql.DB) {
	_, err := db.Exec(`
		TRUNCATE TABLE pull_request_reviewers CASCADE;
		TRUNCATE TABLE pull_requests CASCADE;
		TRUNCATE TABLE users CASCADE;
		TRUNCATE TABLE teams CASCADE;
	`)
	if err != nil {
		t.Logf("Failed to cleanup: %v", err)
	}
	db.Close()
}

func TestCreateTeam(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	repo := New(db)
	ctx := context.Background()

	team := &models.TeamPayload{
		TeamName: "test-team",
		Members: []models.TeamMember{
			{UserID: "u1", Username: "User1", IsActive: true},
			{UserID: "u2", Username: "User2", IsActive: true},
		},
	}

	err := repo.CreateTeam(ctx, team)
	if err != nil {
		t.Fatalf("CreateTeam failed: %v", err)
	}

	gotTeam, err := repo.GetTeam(ctx, "test-team")
	if err != nil {
		t.Fatalf("GetTeam failed: %v", err)
	}

	if len(gotTeam.Members) != 2 {
		t.Errorf("Expected 2 members, got %d", len(gotTeam.Members))
	}
}

func TestDeactivateTeamUsers(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	repo := New(db)
	ctx := context.Background()

	team := &models.TeamPayload{
		TeamName: "deactivate-team",
		Members: []models.TeamMember{
			{UserID: "u1", Username: "User1", IsActive: true},
			{UserID: "u2", Username: "User2", IsActive: true},
		},
	}
	err := repo.CreateTeam(ctx, team)
	if err != nil {
		t.Fatalf("CreateTeam failed: %v", err)
	}

	deactivated, err := repo.DeactivateTeamUsers(ctx, "deactivate-team")
	if err != nil {
		t.Fatalf("DeactivateTeamUsers failed: %v", err)
	}

	if len(deactivated) != 2 {
		t.Errorf("Expected 2 deactivated users, got %d", len(deactivated))
	}

	teamAfter, err := repo.GetTeam(ctx, "deactivate-team")
	if err != nil {
		t.Fatalf("GetTeam failed: %v", err)
	}

	for _, member := range teamAfter.Members {
		if member.IsActive {
			t.Errorf("User %s should be inactive", member.UserID)
		}
	}
}

func TestCreatePRAndMerge(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	repo := New(db)
	ctx := context.Background()

	team := &models.TeamPayload{
		TeamName: "pr-team",
		Members: []models.TeamMember{
			{UserID: "author", Username: "Author", IsActive: true},
		},
	}
	err := repo.CreateTeam(ctx, team)
	if err != nil {
		t.Fatalf("CreateTeam failed: %v", err)
	}

	pr := &models.PRCreatePayload{
		ID:       "pr-1",
		Name:     "Test PR",
		AuthorID: "author",
	}
	err = repo.CreatePR(ctx, pr)
	if err != nil {
		t.Fatalf("CreatePR failed: %v", err)
	}

	err = repo.MergePR(ctx, "pr-1")
	if err != nil {
		t.Fatalf("MergePR failed: %v", err)
	}

	gotPR, err := repo.GetPR(ctx, "pr-1")
	if err != nil {
		t.Fatalf("GetPR failed: %v", err)
	}

	if gotPR.Status != models.StatusMerged {
		t.Errorf("Expected status MERGED, got %s", gotPR.Status)
	}
}
