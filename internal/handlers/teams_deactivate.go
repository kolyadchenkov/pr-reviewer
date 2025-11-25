package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"pr-reviewer-service/internal/models"
)

func (h *Handler) HandleTeamDeactivate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}

	var payload models.TeamDeactivatePayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid json")
		return
	}

	payload.TeamName = strings.TrimSpace(payload.TeamName)
	if payload.TeamName == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "team_name is required")
		return
	}

	ctx := getContext(r)

	// Проверяем существование команды
	exists, err := h.repo.TeamExists(ctx, payload.TeamName)
	if err != nil {
		writeInternal(w)
		return
	}
	if !exists {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "team not found")
		return
	}

	// Деактивируем пользователей команды
	deactivatedIDs, err := h.repo.DeactivateTeamUsers(ctx, payload.TeamName)
	if err != nil {
		writeInternal(w)
		return
	}

	if len(deactivatedIDs) == 0 {
		writeJSON(w, http.StatusOK, models.TeamDeactivateResponse{
			TeamName:         payload.TeamName,
			DeactivatedUsers: []string{},
			ReassignedPRs:    0,
		})
		return
	}

	// Находим открытые PR с деактивированными ревьюверами
	openPRs, err := h.repo.GetOpenPRsWithReviewers(ctx, deactivatedIDs)
	if err != nil {
		writeInternal(w)
		return
	}

	reassignedCount := 0

	// Переназначаем ревьюверов для каждого PR
	for _, pr := range openPRs {
		// Находим деактивированных ревьюверов в этом PR
		deactivatedInPR := make([]string, 0)
		for _, reviewerID := range pr.Reviewers {
			for _, deactivatedID := range deactivatedIDs {
				if reviewerID == deactivatedID {
					deactivatedInPR = append(deactivatedInPR, reviewerID)
					break
				}
			}
		}

		if len(deactivatedInPR) == 0 {
			continue
		}

		// Для каждого деактивированного ревьювера ищем замену
		for _, oldReviewerID := range deactivatedInPR {
			// Получаем команду деактивированного ревьювера (не автора!)
			reviewerTeamName, err := h.repo.GetUserTeam(ctx, oldReviewerID)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					continue
				}
				writeInternal(w)
				return
			}

			// Исключаем автора и уже назначенных ревьюверов
			excludeIDs := []string{pr.AuthorID}
			excludeIDs = append(excludeIDs, pr.Reviewers...)

			replacement, err := h.repo.GetReplacementCandidate(ctx, reviewerTeamName, excludeIDs, pr.ID)
			if err != nil {
				// Если нет кандидата, просто удаляем ревьювера
				if err.Error() == "no candidate found" {
					if err := h.repo.RemoveReviewer(ctx, pr.ID, oldReviewerID); err != nil {
						writeInternal(w)
						return
					}
					reassignedCount++
					continue
				}
				writeInternal(w)
				return
			}

			// Переназначаем
			if err := h.repo.ReassignReviewer(ctx, pr.ID, oldReviewerID, replacement); err != nil {
				writeInternal(w)
				return
			}
			reassignedCount++
		}
	}

	writeJSON(w, http.StatusOK, models.TeamDeactivateResponse{
		TeamName:         payload.TeamName,
		DeactivatedUsers: deactivatedIDs,
		ReassignedPRs:    reassignedCount,
	})
}

