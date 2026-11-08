package handler

import (
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
	"location/internal/repository"
)

// RegisterMobile adds a write-only API. The legacy API remains tailnet/internal.
// Missing configuration fails closed; clients keep their durable local queue.
func (h *Handler) RegisterMobile(mux *http.ServeMux, tokenHash []byte) {
	mux.HandleFunc("PUT /mobile/locations/{id}", func(w http.ResponseWriter, r *http.Request) {
		if !mobileAuthorized(w, r, tokenHash) {
			return
		}
		id := r.PathValue("id")
		parsedID, err := uuid.Parse(id)
		if err != nil || parsedID.String() != id {
			writeErr(r.Context(), w, http.StatusBadRequest, "id must be a canonical UUID")
			return
		}
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4096))
		input, inputErr := legacyMobileInput(id, raw, time.Now())
		if err != nil || inputErr != nil {
			writeErr(r.Context(), w, http.StatusBadRequest, "invalid point")
			return
		}
		loc, err := h.svc.Save(r.Context(), input)
		if errors.Is(err, repository.ErrConflict) {
			writeErr(r.Context(), w, http.StatusConflict, "id belongs to another point")
			return
		}
		if err != nil {
			writeErr(r.Context(), w, http.StatusServiceUnavailable, "storage unavailable; retry the same id")
			return
		}
		// Save returns only after PostgreSQL commits. No ack is sent for failures.
		writeJSON(r.Context(), w, http.StatusOK, struct {
			ID     string `json:"id"`
			Stored bool   `json:"stored"`
		}{loc.ID, true})
	})
}
