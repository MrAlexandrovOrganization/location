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

func NewPostgres(db *pgxpool.Pool) Repository {
	return &postgres{db: db}
}

func (r *postgres) Save(ctx context.Context, loc *model.Location) error {
	err := r.db.QueryRow(ctx, saveSQL,
		loc.ID, loc.Latitude, loc.Longitude, loc.Accuracy,
		loc.LivePeriod, loc.Date, loc.Hidden, loc.Source, loc.RecordedAt, loc.Provider, string(loc.QualityReason),
	).Scan(&loc.Hidden)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrConflict
	}
	return err
}

func (r *postgres) Get(ctx context.Context, id string) (*model.Location, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id, latitude, longitude, accuracy, live_period, date, hidden, source::text, recorded_at, provider, quality_reason
		FROM locations WHERE id = $1`, id)
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
	query := `
		SELECT id, latitude, longitude, accuracy, live_period, date, hidden, source::text, recorded_at, provider, quality_reason
		FROM locations
		WHERE date = $1`
	if !includeHidden {
		query += ` AND NOT hidden`
	}
	query += ` ORDER BY recorded_at ASC`

	rows, err := r.db.Query(ctx, query, date)
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
	tag, err := r.db.Exec(ctx, `DELETE FROM locations WHERE id = $1`, id)
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
		&loc.Provider, &loc.QualityReason}
}
