package repository

import (
	"context"
	_ "embed"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"location/internal/model"
)

const (
	outlierMinimumSamples = 5
	outlierMinimumDays    = 3
	outlierRadiusMetres   = 100
)

//go:embed queries/lock_blacklist.sql
var lockBlacklistSQL string

//go:embed queries/observe_outlier.sql
var observeOutlierSQL string

//go:embed queries/confirm_outlier.sql
var confirmOutlierSQL string

//go:embed queries/add_confirmed_outlier.sql
var addConfirmedOutlierSQL string

// SaveOutlier commits the point, evidence and any confirmed exact rule together.
// A retry/conflicting UUID exits before recording evidence or changing rules.
func (r *postgres) SaveOutlier(ctx context.Context, loc *model.Location, referenceID string) error {
	if !loc.Hidden || referenceID == "" {
		return errors.New("outlier requires a hidden point and a reference")
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, lockBlacklistSQL); err != nil {
		return fmt.Errorf("lock outlier rules: %w", err)
	}
	err = tx.QueryRow(ctx, saveSQL,
		loc.ID, loc.Latitude, loc.Longitude, loc.Accuracy,
		loc.LivePeriod, loc.Date, loc.Hidden, loc.Source, loc.RecordedAt,
	).Scan(&loc.Hidden)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, observeOutlierSQL, loc.ID, referenceID); err != nil {
		return fmt.Errorf("record outlier evidence: %w", err)
	}
	var confirmed bool
	if err := tx.QueryRow(ctx, confirmOutlierSQL, loc.ID,
		outlierMinimumSamples, outlierMinimumDays, outlierRadiusMetres).Scan(&confirmed); err != nil {
		return fmt.Errorf("confirm outlier: %w", err)
	}
	if confirmed {
		if _, err := tx.Exec(ctx, addConfirmedOutlierSQL, loc.ID); err != nil {
			return fmt.Errorf("add confirmed outlier: %w", err)
		}
	}
	return tx.Commit(ctx)
}
