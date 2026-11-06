package repository

import (
	"context"
	_ "embed"
	"github.com/jackc/pgx/v5"
	"location/internal/model"
)

//go:embed queries/lock_measurement.sql
var lockMeasurementSQL string

//go:embed queries/latest_measurement.sql
var latestMeasurementSQL string

// Rule locks precede measurement locks on both write paths. Every observation,
// including duplicates and late arrivals, is retained; only its derived view is hidden.
func prepareMeasurement(ctx context.Context, tx pgx.Tx, loc *model.Location) error {
	if loc.MeasurementKey == "" {
		return nil
	}
	if _, err := tx.Exec(ctx, lockMeasurementSQL, loc.MeasurementKey); err != nil {
		return err
	}
	var latest *int64
	if err := tx.QueryRow(ctx, latestMeasurementSQL, loc.MeasurementKey).Scan(&latest); err != nil {
		return err
	}
	if latest != nil && loc.MeasurementElapsedNS <= *latest {
		loc.Hidden = true
		loc.QualityReason = model.OutOfOrderMeasurement
		if loc.MeasurementElapsedNS == *latest {
			loc.QualityReason = model.DuplicateMeasurement
		}
	}
	return nil
}

func savePoint(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, loc *model.Location) error {
	return q.QueryRow(ctx, saveSQL, loc.ID, loc.Latitude, loc.Longitude, loc.Accuracy, loc.LivePeriod, loc.Date, loc.Hidden, loc.Source, loc.RecordedAt, loc.Provider, string(loc.QualityReason), []byte(loc.RawPayload), loc.RawSHA256, loc.MeasurementKey, loc.MeasurementElapsedNS).Scan(&loc.Hidden)
}
