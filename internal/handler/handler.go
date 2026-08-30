package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"location/internal/model"
	"location/internal/repository"
	"location/internal/service"
)

type Handler struct {
	svc service.Service
}

func New(svc service.Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /locations", h.save)
	mux.HandleFunc("GET /locations", h.list)
	mux.HandleFunc("GET /locations/{id}", h.get)
	mux.HandleFunc("DELETE /locations/{id}", h.delete)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func (h *Handler) save(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Latitude   float64   `json:"latitude"`
		Longitude  float64   `json:"longitude"`
		Accuracy   float32   `json:"accuracy"`
		LivePeriod int       `json:"live_period"`
		Date       string    `json:"date"`
		Source     string    `json:"source"`
		RecordedAt time.Time `json:"recorded_at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(r.Context(), w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if body.Date == "" {
		writeErr(r.Context(), w, http.StatusBadRequest, "date is required")
		return
	}

	loc, err := h.svc.Save(r.Context(), model.CreateInput{
		Latitude:   body.Latitude,
		Longitude:  body.Longitude,
		Accuracy:   body.Accuracy,
		LivePeriod: body.LivePeriod,
		Date:       body.Date,
		Source:     body.Source,
		RecordedAt: body.RecordedAt,
	})
	if err != nil {
		slog.ErrorContext(r.Context(), "save location", "error", err)
		writeErr(r.Context(), w, http.StatusInternalServerError, "save failed")
		return
	}
	writeJSON(r.Context(), w, http.StatusCreated, loc)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	date := r.URL.Query().Get("date")
	if date == "" {
		writeErr(r.Context(), w, http.StatusBadRequest, "date query param is required")
		return
	}
	includeHidden := r.URL.Query().Get("include_hidden") == "true"

	locs, err := h.svc.ListByDate(r.Context(), date, includeHidden)
	if err != nil {
		slog.ErrorContext(r.Context(), "list locations", "error", err)
		writeErr(r.Context(), w, http.StatusInternalServerError, "list failed")
		return
	}
	writeJSON(r.Context(), w, http.StatusOK, locs)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	loc, err := h.svc.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			writeErr(r.Context(), w, http.StatusNotFound, "not found")
			return
		}
		writeErr(r.Context(), w, http.StatusInternalServerError, "get failed")
		return
	}
	writeJSON(r.Context(), w, http.StatusOK, loc)
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	err := h.svc.Delete(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			writeErr(r.Context(), w, http.StatusNotFound, "not found")
			return
		}
		slog.ErrorContext(r.Context(), "delete location", "error", err)
		writeErr(r.Context(), w, http.StatusInternalServerError, "delete failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(ctx context.Context, w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.ErrorContext(ctx, "write json", "error", err)
	}
}

func writeErr(ctx context.Context, w http.ResponseWriter, code int, msg string) {
	writeJSON(ctx, w, code, map[string]string{"error": msg})
}
