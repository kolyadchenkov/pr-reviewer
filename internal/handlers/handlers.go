package handlers

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"pr-reviewer-service/internal/models"
	"pr-reviewer-service/internal/repository"
)

type Handler struct {
	repo repository.Repository
}

func New(repo repository.Repository) *Handler {
	return &Handler{
		repo: repo,
	}
}

func getContext(r *http.Request) context.Context {
	return r.Context()
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
