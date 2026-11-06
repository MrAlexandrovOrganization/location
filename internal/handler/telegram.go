package handler

import (
	"encoding/json"
	"errors"
	"io"
	"location/internal/adapter"
	"location/internal/repository"
	"net/http"
	"time"
)

// Like the legacy API this route is internal/tailnet-only. The bot enforces its
// Telegram user authorization before forwarding location measurements.
func (h *Handler) RegisterTelegram(mux *http.ServeMux) {
	mux.HandleFunc("POST /telegram/measurements", func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 32768))
		var fields map[string]json.RawMessage
		if err != nil || json.Unmarshal(raw, &fields) != nil || fields == nil {
			writeErr(r.Context(), w, 400, "expected a JSON measurement object")
			return
		}
		input := adapter.Telegram(raw, time.Now())
		loc, err := h.svc.Save(r.Context(), input)
		if errors.Is(err, repository.ErrConflict) {
			writeErr(r.Context(), w, 409, "update belongs to another measurement")
			return
		}
		if err != nil {
			writeErr(r.Context(), w, 503, "storage unavailable")
			return
		}
		writeJSON(r.Context(), w, 201, loc)
	})
}
