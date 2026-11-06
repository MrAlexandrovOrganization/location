package repository_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"location/internal/migrate"
	"location/internal/model"
	"location/internal/repository"
	"location/internal/service"
	"location/migrations"
)

func TestConfirmedOutliers(t *testing.T) {
	db := isolatedPostgres(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if err := migrate.Run(ctx, db, migrations.FS); err != nil {
		t.Fatal(err)
	}
	repo := repository.NewPostgres(db)
	svc := service.New(repo, nil, 100)
	first := time.Date(2026, 1, 1, 20, 0, 0, 0, time.UTC)
	// Two UTC dates, three Moscow dates: local-day confirmation must use Moscow.
	times := []time.Time{first, first.Add(10 * time.Minute), first.Add(2 * time.Hour), first.Add(26 * time.Hour), first.Add(26*time.Hour + 10*time.Minute)}
	reset := func(t *testing.T) {
		t.Helper()
		if _, err := db.Exec(ctx, "TRUNCATE location_outlier_observations, locations, location_blacklist RESTART IDENTITY"); err != nil {
			t.Fatal(err)
		}
	}
	count := func(t *testing.T, query string, args ...any) int {
		t.Helper()
		var n int
		if err := db.QueryRow(ctx, query, args...).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	input := func(t *testing.T, id string, when time.Time) model.CreateInput {
		t.Helper()
		ref := &model.Location{ID: id + "-reference", Latitude: 10, Longitude: 20, Accuracy: 5, Date: when.Format("2006-01-02"), Source: "android", RecordedAt: when.Add(-time.Second)}
		if err := repo.Save(ctx, ref); err != nil {
			t.Fatal(err)
		}
		return model.CreateInput{ID: id, Latitude: 30, Longitude: 40, Accuracy: 5, Date: ref.Date, Source: "android", RecordedAt: when}
	}
	reject := func(t *testing.T, id string, when time.Time) model.CreateInput {
		t.Helper()
		in := input(t, id, when)
		p := &model.Location{ID: in.ID, Latitude: in.Latitude, Longitude: in.Longitude, Accuracy: in.Accuracy, Date: in.Date, Source: in.Source, RecordedAt: in.RecordedAt, Hidden: true}
		err := repo.SaveOutlier(ctx, p, id+"-reference", model.SpeedOutlier)
		if err != nil || !p.Hidden {
			t.Fatalf("rejection: %+v %v", p, err)
		}
		return in
	}
	seed := func(t *testing.T, n int) {
		for i := range n {
			reject(t, fmt.Sprintf("sample-%d", i), times[i])
		}
	}

	t.Run("confirmed exact rule and idempotent retry", func(t *testing.T) {
		reset(t)
		seed(t, 4)
		if count(t, "SELECT count(*) FROM location_blacklist") != 0 {
			t.Fatal("rule created before minimum evidence")
		}
		in := reject(t, "fifth", times[4])
		if count(t, "SELECT count(*) FROM location_blacklist WHERE enabled AND radius_m=0 AND latitude=30 AND longitude=40") != 1 {
			t.Fatal("confirmed rule missing")
		}
		for range 3 {
			p, err := svc.Save(ctx, in)
			if err != nil || !p.Hidden {
				t.Fatal("retry", err)
			}
		}
		in.Latitude = 31
		if _, err := svc.Save(ctx, in); !errors.Is(err, repository.ErrConflict) {
			t.Fatalf("conflict=%v", err)
		}
		if count(t, "SELECT count(*) FROM location_outlier_observations") != 5 || count(t, "SELECT count(*) FROM location_blacklist") != 1 {
			t.Fatal("retry/conflict changed evidence or rules")
		}
		near := &model.Location{ID: "near-but-distinct", Latitude: 30.000001, Longitude: 40, Date: "2026-01-03", Source: "android", RecordedAt: times[4]}
		if err := repo.Save(ctx, near); err != nil || near.Hidden {
			t.Fatal("automatic rule grew a radius", err)
		}
	})

	t.Run("one day and old manual hiding are insufficient", func(t *testing.T) {
		reset(t)
		for i := range 5 {
			old := &model.Location{ID: fmt.Sprintf("old-%d", i), Latitude: 30, Longitude: 40, Hidden: true, Date: "2025-12-01", Source: "telegram-bot", RecordedAt: first.AddDate(0, 0, -i-1)}
			if err := repo.Save(ctx, old); err != nil {
				t.Fatal(err)
			}
			reject(t, fmt.Sprintf("same-day-%d", i), first.Add(time.Duration(i)*time.Minute))
		}
		if count(t, "SELECT count(*) FROM location_blacklist") != 0 {
			t.Fatal("old hidden rows or a single day confirmed a rule")
		}
	})

	t.Run("visible visits protect metro and home coordinates", func(t *testing.T) {
		reset(t)
		visit := &model.Location{ID: "known-good", Latitude: 30.0001, Longitude: 40, Date: "2025-12-01", Source: "android", RecordedAt: first.AddDate(0, 0, -10)}
		if err := repo.Save(ctx, visit); err != nil {
			t.Fatal(err)
		}
		seed(t, 5)
		if count(t, "SELECT count(*) FROM location_blacklist") != 0 {
			t.Fatal("automatically banned an area with a visible visit")
		}
		p, err := repo.Get(ctx, visit.ID)
		if err != nil || p.Hidden {
			t.Fatal("known good visit hidden", err)
		}
	})

	for _, radius := range []int{0, 100} {
		t.Run(fmt.Sprintf("manual opt-out radius %d", radius), func(t *testing.T) {
			reset(t)
			if _, err := db.Exec(ctx, "INSERT INTO location_blacklist(latitude,longitude,radius_m,enabled) VALUES (30,40,$1,false)", radius); err != nil {
				t.Fatal(err)
			}
			seed(t, 5)
			if count(t, "SELECT count(*) FROM location_blacklist") != 1 || count(t, "SELECT count(*) FROM location_blacklist WHERE enabled") != 0 {
				t.Fatal("manual opt-out was reactivated or duplicated")
			}
		})
	}

	t.Run("failure rolls back point evidence and rule together", func(t *testing.T) {
		reset(t)
		seed(t, 4)
		if _, err := db.Exec(ctx, `CREATE FUNCTION fail_auto_rule() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic rule failure'; END $$;
CREATE TRIGGER fail_auto_rule BEFORE INSERT ON location_blacklist FOR EACH ROW EXECUTE FUNCTION fail_auto_rule()`); err != nil {
			t.Fatal(err)
		}
		in := input(t, "failed-fifth", times[4])
		if _, err := svc.Save(ctx, in); err == nil {
			t.Fatal("rule failure acknowledged")
		}
		if count(t, "SELECT count(*) FROM locations WHERE id=$1", in.ID) != 0 || count(t, "SELECT count(*) FROM location_outlier_observations") != 4 || count(t, "SELECT count(*) FROM location_blacklist") != 0 {
			t.Fatal("partial transaction survived")
		}
		if _, err := db.Exec(ctx, "DROP TRIGGER fail_auto_rule ON location_blacklist; DROP FUNCTION fail_auto_rule()"); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Save(ctx, in); err != nil {
			t.Fatal("retry after rollback", err)
		}
	})

	t.Run("concurrent threshold creates one rule", func(t *testing.T) {
		reset(t)
		seed(t, 4)
		in := input(t, "concurrent", times[4])
		results := make(chan error, 2)
		for i := range 2 {
			go func() {
				p := &model.Location{ID: fmt.Sprintf("concurrent-%d", i), Latitude: in.Latitude, Longitude: in.Longitude, Accuracy: in.Accuracy, Date: in.Date, Source: in.Source, RecordedAt: in.RecordedAt, Hidden: true}
				results <- repo.SaveOutlier(ctx, p, "concurrent-reference", model.SpeedOutlier)
			}()
		}
		for range 2 {
			if err := <-results; err != nil {
				t.Fatal(err)
			}
		}
		if count(t, "SELECT count(*) FROM location_blacklist") != 1 || count(t, "SELECT count(*) FROM location_outlier_observations") != 6 {
			t.Fatal("concurrent evidence duplicated the rule or lost a point")
		}
	})

	t.Run("offline measurement ignores a newer reference", func(t *testing.T) {
		reset(t)
		in := input(t, "offline", first)
		future := &model.Location{ID: "future", Latitude: 50, Longitude: 60, Date: in.Date, Source: "android", RecordedAt: first.Add(time.Minute)}
		if err := repo.Save(ctx, future); err != nil {
			t.Fatal(err)
		}
		p, err := svc.Save(ctx, in)
		if err != nil || !p.Hidden || count(t, "SELECT count(*) FROM location_outlier_observations") != 1 {
			t.Fatal("newer point bypassed the chronological speed filter", err)
		}
	})
}
