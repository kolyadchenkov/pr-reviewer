package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

const (
	statusOpen   = "OPEN"
	statusMerged = "MERGED"
)

type server struct {
	db     *sql.DB
	random *rand.Rand
}

type errorResponse struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

type teamMember struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	IsActive bool   `json:"is_active"`
}

type teamPayload struct {
	TeamName string       `json:"team_name"`
	Members  []teamMember `json:"members"`
}

type user struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	TeamName string `json:"team_name"`
	IsActive bool   `json:"is_active"`
}

type pullRequest struct {
	ID        string     `json:"pull_request_id"`
	Name      string     `json:"pull_request_name"`
	AuthorID  string     `json:"author_id"`
	Status    string     `json:"status"`
	Reviewers []string   `json:"assigned_reviewers"`
	CreatedAt *time.Time `json:"createdAt"`
	MergedAt  *time.Time `json:"mergedAt"`
}

type pullRequestShort struct {
	ID       string `json:"pull_request_id"`
	Name     string `json:"pull_request_name"`
	AuthorID string `json:"author_id"`
	Status   string `json:"status"`
}

type prCreatePayload struct {
	ID       string `json:"pull_request_id"`
	Name     string `json:"pull_request_name"`
	AuthorID string `json:"author_id"`
}

type prMergePayload struct {
	ID string `json:"pull_request_id"`
}

type prReassignPayload struct {
	ID        string `json:"pull_request_id"`
	OldUserID string `json:"old_user_id"`
}

type userActivePayload struct {
	UserID   string `json:"user_id"`
	IsActive bool   `json:"is_active"`
}

type prResponse struct {
	PR pullRequest `json:"pr"`
}

func main() {
	conn := os.Getenv("DATABASE_URL")
	if conn == "" {
		conn = "postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable"
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	db, err := sql.Open("postgres", conn)
	if err != nil {
		log.Fatalf("connect db: %v", err)
	}
	defer db.Close()

	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(time.Minute * 5)

	if err := db.Ping(); err != nil {
		log.Fatalf("ping db: %v", err)
	}

	srv := &server{
		db:     db,
		random: rand.New(rand.NewSource(time.Now().UnixNano())),
	}

	if err := srv.ensureSchema(); err != nil {
		log.Fatalf("ensure schema: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", srv.handleHealth)
	mux.HandleFunc("/team/add", srv.handleTeamAdd)
	mux.HandleFunc("/team/get", srv.handleTeamGet)
	mux.HandleFunc("/users/setIsActive", srv.handleUserSetActive)
	mux.HandleFunc("/users/getReview", srv.handleUserReviews)
	mux.HandleFunc("/pullRequest/create", srv.handlePRCreate)
	mux.HandleFunc("/pullRequest/merge", srv.handlePRMerge)
	mux.HandleFunc("/pullRequest/reassign", srv.handlePRReassign)

	addr := ":" + port
	log.Printf("server listening on %s", addr)
	if err := http.ListenAndServe(addr, withJSON(mux)); err != nil {
		log.Fatalf("http server: %v", err)
	}
}

func withJSON(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		next.ServeHTTP(w, r)
	})
}

func (s *server) ensureSchema() error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS teams (
			team_name TEXT PRIMARY KEY,
			created_at TIMESTAMPTZ DEFAULT NOW()
		);`,
		`CREATE TABLE IF NOT EXISTS users (
			user_id TEXT PRIMARY KEY,
			username TEXT NOT NULL,
			team_name TEXT NOT NULL REFERENCES teams(team_name) ON DELETE CASCADE,
			is_active BOOLEAN NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS pull_requests (
			pull_request_id TEXT PRIMARY KEY,
			pull_request_name TEXT NOT NULL,
			author_id TEXT NOT NULL REFERENCES users(user_id),
			status TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			merged_at TIMESTAMPTZ
		);`,
		`CREATE TABLE IF NOT EXISTS pull_request_reviewers (
			pr_id TEXT NOT NULL REFERENCES pull_requests(pull_request_id) ON DELETE CASCADE,
			reviewer_id TEXT NOT NULL REFERENCES users(user_id),
			PRIMARY KEY(pr_id, reviewer_id)
		);`,
	}

	for _, stmt := range statements {
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}

	return nil
}

func (s *server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *server) handleTeamAdd(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}

	var payload teamPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid json")
		return
	}

	payload.TeamName = strings.TrimSpace(payload.TeamName)
	if payload.TeamName == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "team_name is required")
		return
	}

	tx, err := s.db.Begin()
	if err != nil {
		writeInternal(w)
		return
	}
	defer tx.Rollback()

	var exists bool
	if err := tx.QueryRow("SELECT EXISTS(SELECT 1 FROM teams WHERE team_name=$1)", payload.TeamName).Scan(&exists); err != nil {
		writeInternal(w)
		return
	}
	if exists {
		writeError(w, http.StatusBadRequest, "TEAM_EXISTS", "team_name already exists")
		return
	}

	if _, err := tx.Exec("INSERT INTO teams(team_name) VALUES($1)", payload.TeamName); err != nil {
		writeInternal(w)
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

		_, err := tx.Exec(`
			INSERT INTO users(user_id, username, team_name, is_active)
			VALUES($1, $2, $3, $4)
			ON CONFLICT (user_id) DO UPDATE SET
				username=EXCLUDED.username,
				team_name=EXCLUDED.team_name,
				is_active=EXCLUDED.is_active
		`, m.UserID, m.Username, payload.TeamName, m.IsActive)
		if err != nil {
			writeInternal(w)
			return
		}
	}

	if err := tx.Commit(); err != nil {
		writeInternal(w)
		return
	}

	team, err := s.getTeam(payload.TeamName)
	if err != nil {
		writeInternal(w)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{"team": team})
}

func (s *server) handleTeamGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}

	name := strings.TrimSpace(r.URL.Query().Get("team_name"))
	if name == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "team_name is required")
		return
	}

	team, err := s.getTeam(name)
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

func (s *server) handleUserSetActive(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}

	var payload userActivePayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid json")
		return
	}

	if strings.TrimSpace(payload.UserID) == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "user_id is required")
		return
	}

	var usr user
	err := s.db.QueryRow(`
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

func (s *server) handlePRCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}

	var payload prCreatePayload
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

	tx, err := s.db.Begin()
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
	`, payload.ID, payload.Name, payload.AuthorID, statusOpen)
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

	pr, err := s.getPR(payload.ID)
	if err != nil {
		writeInternal(w)
		return
	}

	writeJSON(w, http.StatusCreated, prResponse{PR: pr})
}

func (s *server) handlePRMerge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}

	var payload prMergePayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid json")
		return
	}

	payload.ID = strings.TrimSpace(payload.ID)
	if payload.ID == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "pull_request_id is required")
		return
	}

	tx, err := s.db.Begin()
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
	if status != statusMerged {
		if _, err := tx.Exec(`
			UPDATE pull_requests SET status=$1, merged_at=$2 WHERE pull_request_id=$3
		`, statusMerged, now, payload.ID); err != nil {
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

	pr, err := s.getPR(payload.ID)
	if err != nil {
		writeInternal(w)
		return
	}
	writeJSON(w, http.StatusOK, prResponse{PR: pr})
}

func (s *server) handlePRReassign(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}

	var payload prReassignPayload
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

	tx, err := s.db.Begin()
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

	if status == statusMerged {
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

	pr, err := s.getPR(payload.ID)
	if err != nil {
		writeInternal(w)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"pr":          pr,
		"replaced_by": replacement,
	})
}

func (s *server) handleUserReviews(w http.ResponseWriter, r *http.Request) {
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
	if err := s.db.QueryRow("SELECT EXISTS(SELECT 1 FROM users WHERE user_id=$1)", userID).Scan(&exists); err != nil {
		writeInternal(w)
		return
	}
	if !exists {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "user not found")
		return
	}

	rows, err := s.db.Query(`
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

	var prs []pullRequestShort
	for rows.Next() {
		var item pullRequestShort
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

func (s *server) getTeam(name string) (teamPayload, error) {
	var result teamPayload
	row := s.db.QueryRow("SELECT team_name FROM teams WHERE team_name=$1", name)
	if err := row.Scan(&result.TeamName); err != nil {
		return result, err
	}

	rows, err := s.db.Query(`
		SELECT user_id, username, is_active FROM users WHERE team_name=$1 ORDER BY user_id
	`, name)
	if err != nil {
		return result, err
	}
	defer rows.Close()

	for rows.Next() {
		var m teamMember
		if err := rows.Scan(&m.UserID, &m.Username, &m.IsActive); err != nil {
			return result, err
		}
		result.Members = append(result.Members, m)
	}

	return result, rows.Err()
}

func (s *server) getPR(id string) (pullRequest, error) {
	var pr pullRequest
	var createdAt time.Time
	var mergedAt sql.NullTime

	err := s.db.QueryRow(`
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

	rows, err := s.db.Query(`
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

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.WriteHeader(status)
	if payload == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("write json: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	var resp errorResponse
	resp.Error.Code = code
	resp.Error.Message = msg
	writeJSON(w, status, resp)
}

func writeInternal(w http.ResponseWriter) {
	writeError(w, http.StatusInternalServerError, "INTERNAL", "internal error")
}

func methodNotAllowed(w http.ResponseWriter) {
	writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
}
