package main

import (
	"database/sql"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"time"

	_ "github.com/lib/pq"
	"github.com/pressly/goose/v3"

	"pr-reviewer-service/internal/config"
	"pr-reviewer-service/internal/handlers"
)

func main() {
	cfg := config.Load()

	db, err := sql.Open("postgres", cfg.DatabaseURL)
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

	// Применяем миграции
	if err := goose.SetDialect("postgres"); err != nil {
		log.Fatalf("set goose dialect: %v", err)
	}
	
	// Определяем путь к миграциям относительно рабочей директории
	migrationsDir := "migrations"
	if _, err := os.Stat(migrationsDir); os.IsNotExist(err) {
		// Если migrations не найдены, пробуем относительно бинарника
		exe, err := os.Executable()
		if err == nil {
			exeDir := filepath.Dir(exe)
			migrationsDir = filepath.Join(exeDir, "migrations")
		}
	}
	
	if err := goose.Up(db, migrationsDir); err != nil {
		log.Fatalf("run migrations: %v", err)
	}

	h := handlers.New(db, rand.New(rand.NewSource(time.Now().UnixNano())))

	mux := http.NewServeMux()
	mux.HandleFunc("/health", h.HandleHealth)
	mux.HandleFunc("/team/add", h.HandleTeamAdd)
	mux.HandleFunc("/team/get", h.HandleTeamGet)
	mux.HandleFunc("/users/setIsActive", h.HandleUserSetActive)
	mux.HandleFunc("/users/getReview", h.HandleUserReviews)
	mux.HandleFunc("/pullRequest/create", h.HandlePRCreate)
	mux.HandleFunc("/pullRequest/merge", h.HandlePRMerge)
	mux.HandleFunc("/pullRequest/reassign", h.HandlePRReassign)
	mux.HandleFunc("/stats", h.HandleStats)

	addr := ":" + cfg.Port
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

