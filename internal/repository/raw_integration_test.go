package repository_test

import (
	"context"
	"encoding/json"
	"errors"
	"location/internal/adapter"
	"location/internal/migrate"
	"location/internal/model"
	"location/internal/repository"
	"location/internal/service"
	"location/migrations"
	"testing"
	"time"
)

func TestRawMeasurementsAreRetainedAndRetriesRemainImmutable(t *testing.T) {
	db := isolatedPostgres(t)
	ctx := context.Background()
	if err := migrate.Run(ctx, db, migrations.FS); err != nil {
		t.Fatal(err)
	}
	repo := repository.NewPostgres(db)
	svc := service.New(repo, nil, 100)
	ingest := func(ctx context.Context, id string, raw json.RawMessage) (*model.Location, error) {
		input := adapter.Android(raw, time.Now())
		input.ID = id
		return svc.Save(ctx, input)
	}
	when := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	payload := func(elapsed int64, lat any) json.RawMessage {
		raw, err := json.Marshal(map[string]any{"schema_version": 2, "collected_at_ms": when.UnixMilli(), "collected_elapsed_realtime_ns": int64(300 * time.Second), "elapsed_realtime_ns": elapsed, "provider_time_ms": when.Add(4 * time.Minute).UnixMilli(), "provider": "gps", "latitude": lat, "longitude": 20, "accuracy": 7, "installation_id": "e03f3676-049d-4b45-b46c-95501b94aef1", "boot_count": 3, "extras": map[string]any{"synthetic": true}})
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	first := payload(int64(299*time.Second), 10)
	p, err := ingest(ctx, "raw-first", first)
	if err != nil || p.Hidden || !p.RecordedAt.Equal(when.Add(-time.Second)) {
		t.Fatal("first observation", err)
	}
	for range 2 {
		if _, err := ingest(ctx, "raw-first", first); err != nil {
			t.Fatal("same UUID retry", err)
		}
	}
	if _, err := ingest(ctx, "raw-first", payload(int64(299*time.Second), 11)); !errors.Is(err, repository.ErrConflict) {
		t.Fatal("raw data overwritten", err)
	}
	for _, tt := range []struct {
		id     string
		raw    json.RawMessage
		reason model.QualityReason
	}{
		{"raw-duplicate", first, model.DuplicateMeasurement},
		{"raw-late", payload(int64(298*time.Second), 10), model.OutOfOrderMeasurement},
		{"raw-stale", payload(int64(170*time.Second), 10), model.OutOfOrderMeasurement},
		{"raw-invalid", payload(int64(300*time.Second), "NaN"), model.InvalidMeasurement},
	} {
		p, err := ingest(ctx, tt.id, tt.raw)
		if err != nil || !p.Hidden || p.QualityReason != tt.reason {
			t.Fatalf("%s: %+v %v", tt.id, p, err)
		}
		stored, err := repo.Get(ctx, tt.id)
		if err != nil || len(stored.RawPayload) == 0 || stored.RawSHA256 == "" {
			t.Fatal("raw observation lost", err)
		}
	}
	var n int
	if err := db.QueryRow(ctx, "SELECT count(*) FROM locations").Scan(&n); err != nil || n != 5 {
		t.Fatal("observation count", n, err)
	}
	if err := db.QueryRow(ctx, "SELECT count(*) FROM location_outlier_observations").Scan(&n); err != nil || n != 0 {
		t.Fatal("bad metadata became blacklist evidence", err)
	}
	ref, err := repo.GetReference(ctx, when, "gps")
	if err != nil || ref.Point.ID != "raw-first" {
		t.Fatal("hidden raw record poisoned reference", err)
	}
}
