package repository

import (
	"context"
	"time"

	"location/internal/model"
)

type Repository interface {
	Save(ctx context.Context, loc *model.Location) error
	SaveOutlier(ctx context.Context, loc *model.Location, referenceID string, reason model.QualityReason) error
	Get(ctx context.Context, id string) (*model.Location, error)
	GetReference(ctx context.Context, before time.Time, provider string) (*model.Reference, error)
	ListByDate(ctx context.Context, date string, includeHidden bool) ([]*model.Location, error)
	Delete(ctx context.Context, id string) error
}
