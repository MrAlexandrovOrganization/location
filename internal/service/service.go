package service

import (
	"context"
	"errors"
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
	recordedAt = recordedAt.UTC().Truncate(time.Microsecond)
	id := input.ID
	if id == "" {
		id = uuid.New().String()
	}

	source := input.Source
	if source == "" {
		source = "unknown"
	}
	provider := input.Provider
	if provider == "" {
		provider = "unknown"
	}

	loc := &model.Location{
		ID:         id,
		Latitude:   input.Latitude,
		Longitude:  input.Longitude,
		Accuracy:   input.Accuracy,
		LivePeriod: input.LivePeriod,
		Date:       input.Date,
		Source:     source,
		RecordedAt: recordedAt,
		Hidden:     false,
		Provider:   provider,
	}

	// Compare against the last non-hidden point.
	// This handles consecutive outliers: each outlier is always compared
	// to the last valid point, not to the previous outlier.
	reference, err := s.repo.GetReference(ctx, recordedAt, provider)
	if err != nil {
		return nil, fmt.Errorf("find location reference: %w", err)
	}
	var prev *model.Location
	if reference != nil {
		prev = reference.Point
	}
	if input.Accuracy > 500 {
		loc.Hidden, loc.QualityReason = true, model.PoorAccuracy
	} else if prev != nil {
		dt := recordedAt.Sub(prev.RecordedAt).Seconds()
		if dt > 0 {
			dist := haversineMeters(prev.Latitude, prev.Longitude, input.Latitude, input.Longitude)
			// Reported uncertainty is an allowance, not proof of authenticity.
			minimumDistance := math.Max(0, dist-float64(prev.Accuracy)-float64(input.Accuracy))
			speed := minimumDistance / dt
			if speed > s.maxSpeedMS {
				loc.Hidden = true
				loc.QualityReason = model.SpeedOutlier
				if dist >= 5000 && reference.Corroborated {
					loc.QualityReason = model.CorroboratedJump
				} else if dist >= 5000 && reference.CorroboratedPair && speed > math.Max(500, 5*s.maxSpeedMS) {
					// Two agreeing fixes are enough only for an extreme jump. The
					// absolute floor also protects against a very low speed setting.
					loc.QualityReason = model.ExtremeJump
				}
				slog.Info("location hidden: outlier detected",
					"speed_ms", fmt.Sprintf("%.1f", speed),
					"dist_m", fmt.Sprintf("%.0f", dist),
					"dt_s", fmt.Sprintf("%.1f", dt),
				)
			} else if dist >= 5000 {
				freshNetwork := prev.Provider == "network" && prev.Accuracy > 0 && prev.Accuracy <= 250 && dt <= 120
				incomingNetwork := provider == "network" && input.Accuracy > 0 && input.Accuracy <= 250
				if !freshNetwork && !incomingNetwork {
					loc.Hidden, loc.QualityReason = true, model.UncertainJump
				}
			}
		}
	}

	if loc.QualityReason == model.SpeedOutlier || loc.QualityReason == model.CorroboratedJump || loc.QualityReason == model.ExtremeJump {
		err = s.repo.SaveOutlier(ctx, loc, prev.ID, loc.QualityReason)
	} else {
		err = s.repo.Save(ctx, loc)
	}
	if err != nil {
		if errors.Is(err, repository.ErrConflict) && input.ID != "" {
			existing, getErr := s.repo.Get(ctx, id)
			if getErr != nil {
				return nil, getErr
			}
			if existing.Latitude == loc.Latitude && existing.Longitude == loc.Longitude &&
				existing.Accuracy == loc.Accuracy && existing.LivePeriod == loc.LivePeriod &&
				existing.Date == loc.Date && existing.Source == loc.Source && existing.RecordedAt.Equal(loc.RecordedAt) {
				return existing, nil
			}
			return nil, repository.ErrConflict
		}
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
	a = math.Min(1, math.Max(0, a))
	return R * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}
