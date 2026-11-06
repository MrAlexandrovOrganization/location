package handler

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"location/internal/adapter"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"location/internal/repository"
)

func mobileAuthorized(w http.ResponseWriter, r *http.Request, tokenHash []byte) bool {
	w.Header().Set("Cache-Control", "no-store")
	if len(tokenHash) != sha256.Size {
		writeErr(r.Context(), w, http.StatusServiceUnavailable, "mobile ingestion is disabled")
		return false
	}
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") || len(auth) > 512 {
		w.WriteHeader(http.StatusUnauthorized)
		return false
	}
	actual := sha256.Sum256([]byte(strings.TrimPrefix(auth, "Bearer ")))
	if subtle.ConstantTimeCompare(actual[:], tokenHash) != 1 {
		w.WriteHeader(http.StatusUnauthorized)
		return false
	}
	return true
}

func (h *Handler) RegisterMeasurements(mux *http.ServeMux, tokenHash []byte) {
	mux.HandleFunc("PUT /mobile/measurements/{id}", func(w http.ResponseWriter, r *http.Request) {
		if !mobileAuthorized(w, r, tokenHash) {
			return
		}
		id := r.PathValue("id")
		parsed, err := uuid.Parse(id)
		if err != nil || parsed.String() != id {
			writeErr(r.Context(), w, 400, "id must be a canonical UUID")
			return
		}
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 65536))
		var fields map[string]json.RawMessage
		if err != nil || json.Unmarshal(raw, &fields) != nil || fields == nil {
			writeErr(r.Context(), w, 400, "expected a JSON measurement object")
			return
		}
		input := adapter.Android(raw, time.Now())
		input.ID = id
		loc, err := h.svc.Save(r.Context(), input)
		if errors.Is(err, repository.ErrConflict) {
			writeErr(r.Context(), w, 409, "id belongs to another measurement")
			return
		}
		if err != nil {
			writeErr(r.Context(), w, 503, "storage unavailable; retry the same id")
			return
		}
		writeJSON(r.Context(), w, 200, struct {
			ID     string `json:"id"`
			Stored bool   `json:"stored"`
		}{loc.ID, true})
	})
}
