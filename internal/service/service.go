package service

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/google/uuid"
	"location/internal/metrics"
	"location/internal/model"
	"location/internal/repository"
)

type Service interface {
	Save(ctx context.Context, input model.CreateInput) (*model.Location, error)
	Get(ctx context.Context, id string) (*model.Location, error)
	ListByDate(ctx context.Context, date string, includeHidden bool) ([]*model.Location, error)
	Delete(ctx context.Context, id string) error
}

type svc struct {
	repo       repository.Repository
	metrics    *metrics.Metrics
	maxSpeedMS float64
}

func New(repo repository.Repository, m *metrics.Metrics, maxSpeedMS float64) Service {
	return &svc{repo: repo, metrics: m, maxSpeedMS: maxSpeedMS}
}

func (s *svc) Save(ctx context.Context, input model.CreateInput) (*model.Location, error) {
	recordedAt := input.RecordedAt
	if recordedAt.IsZero() {
		recordedAt = time.Now()
	}

	loc := &model.Location{
		ID:         uuid.New().String(),
		Latitude:   input.Latitude,
		Longitude:  input.Longitude,
		Accuracy:   input.Accuracy,
		LivePeriod: input.LivePeriod,
		Date:       input.Date,
		RecordedAt: recordedAt,
		Hidden:     false,
	}

	// Compare against the last non-hidden point.
	// This handles consecutive outliers: each outlier is always compared
	// to the last valid point, not to the previous outlier.
	if prev, err := s.repo.GetLatestValid(ctx, input.Date); err == nil && prev != nil {
		dt := recordedAt.Sub(prev.RecordedAt).Seconds()
		if dt > 0 {
			dist := haversineMeters(prev.Latitude, prev.Longitude, input.Latitude, input.Longitude)
			speed := dist / dt
			if speed > s.maxSpeedMS {
				loc.Hidden = true
				slog.Info("location hidden: outlier detected",
					"speed_ms", fmt.Sprintf("%.1f", speed),
					"dist_m", fmt.Sprintf("%.0f", dist),
					"dt_s", fmt.Sprintf("%.1f", dt),
					"lat", input.Latitude,
					"lon", input.Longitude,
				)
			}
		}
	}

	if err := s.repo.Save(ctx, loc); err != nil {
		return nil, fmt.Errorf("save location: %w", err)
	}
	if s.metrics != nil {
		s.metrics.RecordSave(ctx, input.LivePeriod > 0, loc.Hidden)
	}
	return loc, nil
}

func (s *svc) Get(ctx context.Context, id string) (*model.Location, error) {
	return s.repo.Get(ctx, id)
}

func (s *svc) ListByDate(ctx context.Context, date string, includeHidden bool) ([]*model.Location, error) {
	locs, err := s.repo.ListByDate(ctx, date, includeHidden)
	if err != nil {
		return nil, err
	}
	if locs == nil {
		locs = []*model.Location{}
	}
	return locs, nil
}

func (s *svc) Delete(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
}

func haversineMeters(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6_371_000
	phi1 := lat1 * math.Pi / 180
	phi2 := lat2 * math.Pi / 180
	dLat := (lat2 - lat1) * math.Pi / 180
	dLon := (lon2 - lon1) * math.Pi / 180
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(phi1)*math.Cos(phi2)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return R * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}
