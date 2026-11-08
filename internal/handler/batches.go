package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
	"location/internal/model"
	"location/internal/repository"
)

const mobileBatchMaxPoints = 100
const mobileBatchMaxBytes = 256 * 1024

type mobileBatchPoint struct {
	ID     string `json:"id"`
	Format string `json:"format"`
	// String preserves the original UTF-8 body/hash across single/batch retries.
	Payload string `json:"payload"`
}

type mobileBatchResult struct {
	ID     string `json:"id"`
	Stored bool   `json:"stored"`
	Status int    `json:"status"`
}

func (h *Handler) RegisterBatches(mux *http.ServeMux, tokenHash []byte) {
	mux.HandleFunc("POST /mobile/batches", func(w http.ResponseWriter, r *http.Request) {
		if !mobileAuthorized(w, r, tokenHash) {
			return
		}
		var body struct {
			Points []mobileBatchPoint `json:"points"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, mobileBatchMaxBytes))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&body) != nil || decoder.Decode(new(any)) != io.EOF || len(body.Points) == 0 || len(body.Points) > mobileBatchMaxPoints {
			writeErr(r.Context(), w, 400, "invalid mobile batch")
			return
		}
		ids := make(map[string]bool, len(body.Points))
		for _, p := range body.Points {
			id, err := uuid.Parse(p.ID)
			if err != nil || id.String() != p.ID || ids[p.ID] || (p.Format != "measurement" && p.Format != "location") {
				writeErr(r.Context(), w, 400, "invalid mobile batch item")
				return
			}
			ids[p.ID] = true
		}
		select {
		case h.batchSlots <- struct{}{}:
			defer func() { <-h.batchSlots }()
		default:
			w.Header().Set("Retry-After", "1")
			writeErr(r.Context(), w, 503, "mobile batch busy; retry the same ids")
			return
		}
		// Return committed prefixes before the regular ingress/client deadlines.
		_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(12 * time.Second))
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cancel()
		results := make([]mobileBatchResult, 0, len(body.Points))
		for _, p := range body.Points {
			if r.Context().Err() != nil {
				return
			}
			if ctx.Err() != nil {
				break
			}
			var input model.CreateInput
			var err error
			if p.Format == "measurement" {
				input, err = measurementInput(p.ID, []byte(p.Payload), time.Now())
			} else {
				input, err = legacyMobileInput(p.ID, []byte(p.Payload), time.Now())
			}
			status := http.StatusOK
			if err != nil {
				status = 400
			} else {
				var saved *model.Location
				saved, err = h.svc.Save(ctx, input)
				if errors.Is(err, repository.ErrConflict) {
					status = 409
				} else if err != nil || saved == nil || saved.ID != p.ID {
					status = 503
				}
			}
			results = append(results, mobileBatchResult{ID: p.ID, Stored: status == 200, Status: status})
			if status == 503 {
				break
			}
		}
		if len(results) == 0 {
			writeErr(r.Context(), w, 503, "mobile batch time budget expired; retry the same ids")
			return
		}
		writeJSON(r.Context(), w, 200, struct {
			Results []mobileBatchResult `json:"results"`
		}{results})
	})
}
