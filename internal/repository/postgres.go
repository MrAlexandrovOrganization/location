package repository

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"location/internal/model"
)

type postgres struct {
	db *pgxpool.Pool
}

//go:embed queries/save.sql
var saveSQL string

//go:embed queries/reference.sql
var referenceSQL string

//go:embed queries/lock_points.sql
var lockPointsSQL string

//go:embed queries/get.sql
var getSQL string

//go:embed queries/list.sql
var listSQL string

//go:embed queries/delete.sql
var deleteSQL string

func NewPostgres(db *pgxpool.Pool) Repository {
	return &postgres{db: db}
}

func (r *postgres) Save(ctx context.Context, loc *model.Location) error {
	var err error
	if loc.MeasurementKey == "" {
		err = savePoint(ctx, r.db, loc)
	} else {
		tx, e := r.db.Begin(ctx)
		if e != nil {
			return e
		}
		defer tx.Rollback(ctx)
		if _, e = tx.Exec(ctx, lockPointsSQL); e != nil {
			return e
		}
		if e = prepareMeasurement(ctx, tx, loc); e != nil {
			return e
		}
		err = savePoint(ctx, tx, loc)
		if err == nil {
			return tx.Commit(ctx)
		}
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrConflict
	}
	return err
}

func (r *postgres) Get(ctx context.Context, id string) (*model.Location, error) {
	row := r.db.QueryRow(ctx, getSQL, id)
	return scanLocation(row)
}

func (r *postgres) GetReference(ctx context.Context, before time.Time, provider string) (*model.Reference, error) {
	return scanReference(r.db.QueryRow(ctx, referenceSQL, before, provider))
}

func scanReference(row scanner) (*model.Reference, error) {
	ref := &model.Reference{Point: &model.Location{}}
	fields := append(locationFields(ref.Point), &ref.Corroborated, &ref.CorroboratedPair)
	err := row.Scan(fields...)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return ref, err
}

func (r *postgres) ListByDate(ctx context.Context, date string, includeHidden bool) ([]*model.Location, error) {
	rows, err := r.db.Query(ctx, listSQL, date, includeHidden)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var locs []*model.Location
	for rows.Next() {
		loc, err := scanLocation(rows)
		if err != nil {
			return nil, err
		}
		locs = append(locs, loc)
	}
	return locs, rows.Err()
}

func (r *postgres) Delete(ctx context.Context, id string) error {
	tag, err := r.db.Exec(ctx, deleteSQL, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanLocation(row scanner) (*model.Location, error) {
	var loc model.Location
	err := row.Scan(locationFields(&loc)...)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("scan location: %w", err)
	}
	return &loc, nil
}

func locationFields(loc *model.Location) []any {
	return []any{&loc.ID, &loc.Latitude, &loc.Longitude, &loc.Accuracy,
		&loc.LivePeriod, &loc.Date, &loc.Hidden, &loc.Source, &loc.RecordedAt,
		&loc.Provider, &loc.QualityReason, &loc.RawPayload, &loc.RawSHA256, &loc.MeasurementKey, &loc.MeasurementElapsedNS}
}
