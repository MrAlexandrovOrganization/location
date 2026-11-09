package repository_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"location/internal/adapter"
	"location/internal/migrate"
	"location/internal/model"
	"location/internal/repository"
	"location/internal/service"
	"location/migrations"
)

func TestQualityWarningsRemainVisibleWithoutReopeningManualHides(t *testing.T) {
	db := isolatedPostgres(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := migrate.Run(ctx, db, migrations.FS); err != nil {
		t.Fatal(err)
	}
	repo := repository.NewPostgres(db)
	svc := service.New(repo, nil, 100)
	callback := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	var lastRaw json.RawMessage
	for i, elapsed := range []time.Duration{9 * time.Minute, 15 * time.Minute, 17*time.Minute + 30*time.Second, 20 * time.Minute} {
		accuracy := 200
		if i == 3 {
			accuracy = 1800
		}
		// Several historical fixes delivered in one callback window. Receipt an
		// hour later must not change their normalized measurement timestamps.
		raw, err := json.Marshal(map[string]any{
			"schema_version": 2, "collected_at_ms": callback.UnixMilli(),
			"collected_elapsed_realtime_ns": int64(20 * time.Minute), "elapsed_realtime_ns": int64(elapsed),
			"installation_id": "e03f3676-049d-4b45-b46c-95501b94aef1", "boot_count": 3,
			"provider": "network", "latitude": 10, "longitude": 20, "accuracy": accuracy,
		})
		if err != nil {
			t.Fatal(err)
		}
		lastRaw = raw
		in := adapter.Android(raw, callback.Add(time.Hour))
		in.ID = fmt.Sprintf("warning-%d", i)
		p, err := svc.Save(ctx, in)
		want := model.StaleMeasurement
		if i == 3 {
			want = model.PoorAccuracy
		}
		if err != nil || p.Hidden || p.QualityReason != want || !p.RecordedAt.Equal(callback.Add(elapsed-20*time.Minute)) {
			t.Fatal("quality warning hidden or measurement time changed", p, err)
		}
		stored, err := repo.Get(ctx, in.ID)
		if err != nil || stored.Hidden || stored.RawSHA256 != in.RawSHA256 || len(stored.RawPayload) == 0 {
			t.Fatal("quality warning or immutable raw data lost", stored, err)
		}
	}
	if ref, err := repo.GetReference(ctx, callback.Add(time.Second), "network"); err != nil || ref != nil {
		t.Fatal("warning became a trusted reference", ref, err)
	}

	// A retry must preserve a prior manual hide, even though newly arriving
	// points with the same quality warning are now visible by default.
	if _, err := db.Exec(ctx, "UPDATE locations SET hidden=true WHERE id='warning-3'"); err != nil {
		t.Fatal(err)
	}
	retry := adapter.Android(lastRaw, callback.Add(2*time.Hour))
	retry.ID = "warning-3"
	p, err := svc.Save(ctx, retry)
	if err != nil || !p.Hidden || p.QualityReason != model.PoorAccuracy || p.RawSHA256 != retry.RawSHA256 {
		t.Fatal("retry reopened a manual hide or changed raw identity", p, err)
	}

	if _, err := db.Exec(ctx, "INSERT INTO location_blacklist(latitude,longitude) VALUES(30,40)"); err != nil {
		t.Fatal(err)
	}
	for _, reason := range []model.QualityReason{model.PoorAccuracy, model.StaleMeasurement} {
		p, err := svc.Save(ctx, model.CreateInput{ID: string(reason), Latitude: 30, Longitude: 40, Accuracy: 200, Source: "android", Date: "2026-01-01", RecordedAt: callback, Provider: "network", InputReason: reason})
		if err != nil || !p.Hidden || p.QualityReason != reason {
			t.Fatal("quality warning bypassed the coordinate blacklist", p, err)
		}
	}
	var observations, rules int
	if err := db.QueryRow(ctx, "SELECT (SELECT count(*) FROM location_outlier_observations), (SELECT count(*) FROM location_blacklist)").Scan(&observations, &rules); err != nil || observations != 0 || rules != 1 {
		t.Fatal("quality warnings created blacklist evidence or rules", observations, rules, err)
	}
}
