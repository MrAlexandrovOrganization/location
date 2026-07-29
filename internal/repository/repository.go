package repository

import (
	"context"

	"location/internal/model"
)

type Repository interface {
	Save(ctx context.Context, loc *model.Location) error
	Get(ctx context.Context, id string) (*model.Location, error)
	GetLatestValid(ctx context.Context, date string) (*model.Location, error)
	ListByDate(ctx context.Context, date string, includeHidden bool) ([]*model.Location, error)
	Delete(ctx context.Context, id string) error
}
