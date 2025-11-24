package handlers

import (
	"database/sql"
	"encoding/json"
	"log"
	"math/rand"
	"net/http"

	"pr-reviewer-service/internal/models"
)

type Handler struct {
	db     *sql.DB
	random *rand.Rand
}

func New(db *sql.DB, random *rand.Rand) *Handler {
	return &Handler{
		db:     db,
		random: random,
	}
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if payload == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("write json: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	var resp models.ErrorResponse
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

