package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"location/internal/model"
)

type postgres struct {
	db *pgxpool.Pool
}

func NewPostgres(db *pgxpool.Pool) Repository {
	return &postgres{db: db}
}

func (r *postgres) Save(ctx context.Context, loc *model.Location) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO locations (id, latitude, longitude, accuracy, live_period, date, hidden, source, recorded_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8::location_source, $9)`,
		loc.ID, loc.Latitude, loc.Longitude, loc.Accuracy,
		loc.LivePeriod, loc.Date, loc.Hidden, loc.Source, loc.RecordedAt,
	)
	return err
}

func (r *postgres) Get(ctx context.Context, id string) (*model.Location, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id, latitude, longitude, accuracy, live_period, date, hidden, source::text, recorded_at
		FROM locations WHERE id = $1`, id)
	return scanLocation(row)
}

// GetLatestValid returns the most recent non-hidden point for the given date.
// Returns nil (no error) when no valid point exists yet.
func (r *postgres) GetLatestValid(ctx context.Context, date string) (*model.Location, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id, latitude, longitude, accuracy, live_period, date, hidden, source::text, recorded_at
		FROM locations
		WHERE date = $1 AND NOT hidden
		ORDER BY recorded_at DESC
		LIMIT 1`, date)
	loc, err := scanLocation(row)
	if err == ErrNotFound {
		return nil, nil
	}
	return loc, err
}

func (r *postgres) ListByDate(ctx context.Context, date string, includeHidden bool) ([]*model.Location, error) {
	query := `
		SELECT id, latitude, longitude, accuracy, live_period, date, hidden, source::text, recorded_at
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
	var recordedAt time.Time
	err := row.Scan(
		&loc.ID, &loc.Latitude, &loc.Longitude, &loc.Accuracy,
		&loc.LivePeriod, &loc.Date, &loc.Hidden, &loc.Source, &recordedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("scan location: %w", err)
	}
	loc.RecordedAt = recordedAt
	return &loc, nil
}
