package handler

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"location/internal/model"
	"location/internal/repository"
)

// RegisterMobile adds a write-only API. The legacy API remains tailnet/internal.
// Missing configuration fails closed; clients keep their durable local queue.
func (h *Handler) RegisterMobile(mux *http.ServeMux, tokenHash []byte) {
	mux.HandleFunc("PUT /mobile/locations/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if len(tokenHash) != sha256.Size {
			writeErr(r.Context(), w, http.StatusServiceUnavailable, "mobile ingestion is disabled")
			return
		}
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") || len(auth) > 512 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		actual := sha256.Sum256([]byte(strings.TrimPrefix(auth, "Bearer ")))
		if subtle.ConstantTimeCompare(actual[:], tokenHash) != 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		id := r.PathValue("id")
		parsedID, err := uuid.Parse(id)
		if err != nil || parsedID.String() != id {
			writeErr(r.Context(), w, http.StatusBadRequest, "id must be a canonical UUID")
			return
		}
		var body struct {
			Latitude   *float64  `json:"latitude"`
			Longitude  *float64  `json:"longitude"`
			Accuracy   *float32  `json:"accuracy"`
			RecordedAt time.Time `json:"recorded_at"`
			Provider   string    `json:"provider"`
		}
		r.Body = http.MaxBytesReader(w, r.Body, 4096)
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&body); err != nil {
			writeErr(r.Context(), w, http.StatusBadRequest, "invalid point")
			return
		}
		if decoder.Decode(new(any)) != io.EOF || body.Latitude == nil || body.Longitude == nil || body.Accuracy == nil ||
			math.IsNaN(*body.Latitude) || math.IsInf(*body.Latitude, 0) || math.Abs(*body.Latitude) > 90 ||
			math.IsNaN(*body.Longitude) || math.IsInf(*body.Longitude, 0) || math.Abs(*body.Longitude) > 180 ||
			math.IsNaN(float64(*body.Accuracy)) || math.IsInf(float64(*body.Accuracy), 0) || *body.Accuracy < 0 || !model.ValidProvider(body.Provider) ||
			body.RecordedAt.Before(time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)) || body.RecordedAt.After(time.Now().Add(24*time.Hour)) {
			writeErr(r.Context(), w, http.StatusBadRequest, "invalid point")
			return
		}
		loc, err := h.svc.Save(r.Context(), model.CreateInput{
			ID: id, Latitude: *body.Latitude, Longitude: *body.Longitude, Accuracy: *body.Accuracy,
			RecordedAt: body.RecordedAt, Date: body.RecordedAt.UTC().Format("2006-01-02"), Source: "android", Provider: body.Provider,
		})
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
