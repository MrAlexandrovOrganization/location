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
func (r *postgres) SaveOutlier(ctx context.Context, loc *model.Location, referenceID string, reason model.QualityReason) error {
	if !loc.Hidden || referenceID == "" || (reason != model.SpeedOutlier && reason != model.CorroboratedJump) {
		return errors.New("outlier requires a hidden point and a reference")
	}
	loc.QualityReason = reason
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
		loc.LivePeriod, loc.Date, loc.Hidden, loc.Source, loc.RecordedAt, loc.Provider, string(loc.QualityReason),
	).Scan(&loc.Hidden)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	// Recheck after acquiring the rule lock and inserting the new hidden row.
	// A retry/conflict has already exited; it must not create any new evidence.
	reference, err := scanReference(tx.QueryRow(ctx, referenceSQL, loc.RecordedAt, loc.Provider))
	if err != nil {
		return fmt.Errorf("recheck outlier reference: %w", err)
	}
	if reference == nil || reference.Point.ID != referenceID ||
		(reason == model.CorroboratedJump && !reference.Corroborated) {
		return errors.New("location reference changed; retry the same point")
	}
	if _, err := tx.Exec(ctx, observeOutlierSQL, loc.ID, referenceID); err != nil {
		return fmt.Errorf("record outlier evidence: %w", err)
	}
	var confirmed bool
	if err := tx.QueryRow(ctx, confirmOutlierSQL, loc.ID,
		outlierMinimumSamples, outlierMinimumDays, outlierRadiusMetres, reason == model.CorroboratedJump).Scan(&confirmed); err != nil {
		return fmt.Errorf("confirm outlier: %w", err)
	}
	if confirmed {
		if _, err := tx.Exec(ctx, addConfirmedOutlierSQL, loc.ID); err != nil {
			return fmt.Errorf("add confirmed outlier: %w", err)
		}
	}
	return tx.Commit(ctx)
}
