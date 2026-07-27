package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"location/internal/metrics"
	"location/internal/model"
	"location/internal/repository"
)

type Service interface {
	Save(ctx context.Context, input model.CreateInput) (*model.Location, error)
	Get(ctx context.Context, id string) (*model.Location, error)
	ListByDate(ctx context.Context, date string) ([]*model.Location, error)
	Delete(ctx context.Context, id string) error
}

type svc struct {
	repo    repository.Repository
	metrics *metrics.Metrics
}

func New(repo repository.Repository, m *metrics.Metrics) Service {
	return &svc{repo: repo, metrics: m}
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
	}
	if err := s.repo.Save(ctx, loc); err != nil {
		return nil, fmt.Errorf("save location: %w", err)
	}
	if s.metrics != nil {
		s.metrics.RecordSave(ctx, input.LivePeriod > 0)
	}
	return loc, nil
}

func (s *svc) Get(ctx context.Context, id string) (*model.Location, error) {
	return s.repo.Get(ctx, id)
}

func (s *svc) ListByDate(ctx context.Context, date string) ([]*model.Location, error) {
	locs, err := s.repo.ListByDate(ctx, date)
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
