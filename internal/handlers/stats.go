package handlers

import (
	"net/http"
)

func (h *Handler) HandleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}

	ctx := getContext(r)
	resp, err := h.repo.GetStats(ctx)
	if err != nil {
		writeInternal(w)
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

