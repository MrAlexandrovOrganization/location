package repository_test

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"location/internal/migrate"
	"location/internal/model"
	"location/internal/repository"
	"location/internal/service"
	"location/migrations"
)

func TestPostgresBlacklist(t *testing.T) {
	db := isolatedPostgres(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := migrate.Run(ctx, db, migrations.FS); err != nil {
		t.Fatal(err)
	}
	repo := repository.NewPostgres(db)
	reset := func(t *testing.T) {
		t.Helper()
		if _, err := db.Exec(ctx, "TRUNCATE location_outlier_observations, locations, location_blacklist RESTART IDENTITY"); err != nil {
			t.Fatal(err)
		}
	}
	save := func(t *testing.T, id string, lat, lon float64, hidden bool) *model.Location {
		t.Helper()
		point := &model.Location{ID: id, Latitude: lat, Longitude: lon, Accuracy: 5, Date: "2026-01-01", Source: "android", Hidden: hidden, RecordedAt: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
		if err := repo.Save(ctx, point); err != nil {
			t.Fatal(err)
		}
		return point
	}
	hidden := func(t *testing.T, id string) bool {
		t.Helper()
		point, err := repo.Get(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		return point.Hidden
	}
	add := func(t *testing.T, lat, lon, radius float64, enabled bool) int64 {
		t.Helper()
		var id int64
		if err := db.QueryRow(ctx, "INSERT INTO location_blacklist(latitude,longitude,radius_m,enabled) VALUES ($1,$2,$3,$4) RETURNING id", lat, lon, radius, enabled).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	exec := func(t *testing.T, sql string, args ...any) {
		t.Helper()
		if _, err := db.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("exact match backfills history and reports actual saved state", func(t *testing.T) {
		reset(t)
		save(t, "history", 10, 20, false)
		save(t, "manual", 11, 21, true)
		add(t, 10, 20, 0, true)
		if !hidden(t, "history") || !hidden(t, "manual") {
			t.Fatal("historical/manual visibility lost")
		}
		point := save(t, "retry-point", 10, 20, false)
		if !point.Hidden {
			t.Fatal("Save did not return the trigger's hidden flag")
		}
		if save(t, "near-but-not-equal", 10.000001, 20, false).Hidden {
			t.Fatal("zero radius matched a different coordinate")
		}
		exec(t, "UPDATE locations SET hidden=false WHERE id=$1", "history")
		if !hidden(t, "history") {
			t.Fatal("matching point was manually unhidden")
		}
		exec(t, "UPDATE locations SET latitude=$1 WHERE id=$2", 10, "near-but-not-equal")
		if !hidden(t, "near-but-not-equal") {
			t.Fatal("coordinate update bypassed blacklist")
		}
		svc := service.New(repo, nil, 100)
		again, err := svc.Save(ctx, model.CreateInput{ID: point.ID, Latitude: point.Latitude, Longitude: point.Longitude, Accuracy: point.Accuracy, Date: point.Date, Source: point.Source, RecordedAt: point.RecordedAt})
		if err != nil || !again.Hidden {
			t.Fatalf("retry: %+v %v", again, err)
		}
		visible, err := repo.ListByDate(ctx, point.Date, false)
		if err != nil || len(visible) != 0 {
			t.Fatalf("blacklisted point leaked into list: %v %v", visible, err)
		}
		latest, err := repo.GetLatestValid(ctx, point.Date, point.RecordedAt.Add(time.Second))
		if err != nil || latest != nil {
			t.Fatal("blacklisted point was used by the velocity filter")
		}
	})

	t.Run("radius dateline and polar coordinates", func(t *testing.T) {
		reset(t)
		add(t, 0, 0, 1000, true)
		add(t, 10, 179.999, 500, true)
		add(t, 89.999, 0, 300, true)
		for _, test := range []struct {
			id       string
			lat, lon float64
			want     bool
		}{
			{"inside", 0.005, 0, true}, {"outside", 0.02, 0, false},
			{"dateline", 10, -179.999, true}, {"outside-dateline", 10, -179.99, false},
			{"pole", 89.999, 180, true}, {"outside-pole", 89.99, 180, false},
		} {
			if p := save(t, test.id, test.lat, test.lon, false); p.Hidden != test.want {
				t.Fatalf("%s hidden=%v want %v", test.id, p.Hidden, test.want)
			}
		}
	})

	t.Run("disabled edited and deleted rules preserve prior hiding", func(t *testing.T) {
		reset(t)
		id := add(t, 10, 20, 0, false)
		if save(t, "disabled", 10, 20, false).Hidden {
			t.Fatal("disabled rule hid a point")
		}
		exec(t, "UPDATE location_blacklist SET enabled=true WHERE id=$1", id)
		if !hidden(t, "disabled") {
			t.Fatal("enabling rule did not backfill history")
		}
		save(t, "new-centre", 30, 40, false)
		exec(t, "UPDATE location_blacklist SET latitude=$1,longitude=$2 WHERE id=$3", 30, 40, id)
		if !hidden(t, "new-centre") || !hidden(t, "disabled") {
			t.Fatal("rule edit lost hiding")
		}
		exec(t, "UPDATE location_blacklist SET enabled=false WHERE id=$1", id)
		if save(t, "after-disable", 30, 40, false).Hidden {
			t.Fatal("disabled rule still matched")
		}
		exec(t, "DELETE FROM location_blacklist WHERE id=$1", id)
		if !hidden(t, "new-centre") || !hidden(t, "disabled") {
			t.Fatal("rule removal reopened hidden history")
		}
	})

	t.Run("rule and historical changes roll back together", func(t *testing.T) {
		reset(t)
		save(t, "history", 10, 20, false)
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		if _, err := tx.Exec(ctx, "INSERT INTO location_blacklist(latitude,longitude) VALUES ($1,$2)", 10, 20); err != nil {
			t.Fatal(err)
		}
		var inside bool
		if err := tx.QueryRow(ctx, "SELECT hidden FROM locations WHERE id=$1", "history").Scan(&inside); err != nil || !inside {
			t.Fatalf("backfill not in transaction: %v", err)
		}
		if hidden(t, "history") {
			t.Fatal("uncommitted rule was visible outside transaction")
		}
		if err := tx.Rollback(ctx); err != nil {
			t.Fatal(err)
		}
		if hidden(t, "history") || save(t, "after-rollback", 10, 20, false).Hidden {
			t.Fatal("rolled-back rule still affected points")
		}
	})

	t.Run("reject invalid rule coordinates and radii", func(t *testing.T) {
		reset(t)
		for _, v := range [][3]float64{{91, 0, 0}, {0, 181, 0}, {0, 0, -1}, {0, 0, 100001}, {math.NaN(), 0, 0}, {0, math.Inf(1), 0}, {0, 0, math.NaN()}} {
			if _, err := db.Exec(ctx, "INSERT INTO location_blacklist(latitude,longitude,radius_m) VALUES ($1,$2,$3)", v[0], v[1], v[2]); err == nil {
				t.Fatalf("accepted invalid rule: %v", v)
			}
		}
	})

	t.Run("concurrent point commit is included by rule backfill", func(t *testing.T) {
		reset(t)
		point, rule := blacklistTransactions(t, db, ctx)
		defer func() { _ = point.Rollback(ctx); _ = rule.Rollback(ctx) }()
		if _, err := point.Exec(ctx, "INSERT INTO locations(id,latitude,longitude,date) VALUES ('concurrent',10,20,'2026-01-01')"); err != nil {
			t.Fatal(err)
		}
		var pid int
		if err := rule.QueryRow(ctx, "SELECT pg_backend_pid()").Scan(&pid); err != nil {
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() {
			_, err := rule.Exec(ctx, "INSERT INTO location_blacklist(latitude,longitude) VALUES (10,20)")
			done <- err
		}()
		waitForBlacklistLock(t, db, pid)
		if err := point.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		if err := <-done; err != nil {
			t.Fatal(err)
		}
		if err := rule.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		if !hidden(t, "concurrent") {
			t.Fatal("rule missed a concurrent committed point")
		}
	})

	t.Run("new point observes a concurrently committed rule", func(t *testing.T) {
		reset(t)
		rule, point := blacklistTransactions(t, db, ctx)
		defer func() { _ = rule.Rollback(ctx); _ = point.Rollback(ctx) }()
		if _, err := rule.Exec(ctx, "INSERT INTO location_blacklist(latitude,longitude) VALUES (10,20)"); err != nil {
			t.Fatal(err)
		}
		var pid int
		if err := point.QueryRow(ctx, "SELECT pg_backend_pid()").Scan(&pid); err != nil {
			t.Fatal(err)
		}
		type result struct {
			hidden bool
			err    error
		}
		done := make(chan result, 1)
		go func() {
			var h bool
			err := point.QueryRow(ctx, "INSERT INTO locations(id,latitude,longitude,date) VALUES ('concurrent',10,20,'2026-01-01') RETURNING hidden").Scan(&h)
			done <- result{h, err}
		}()
		waitForBlacklistLock(t, db, pid)
		if err := rule.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		got := <-done
		if got.err != nil {
			t.Fatal(got.err)
		}
		if err := point.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		if !got.hidden || !hidden(t, "concurrent") {
			t.Fatal("new point missed a concurrently committed rule")
		}
	})
}

func blacklistTransactions(t *testing.T, db *pgxpool.Pool, ctx context.Context) (pgx.Tx, pgx.Tx) {
	t.Helper()
	first, err := db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		t.Fatal(err)
	}
	second, err := db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		_ = first.Rollback(ctx)
		t.Fatal(err)
	}
	return first, second
}

func waitForBlacklistLock(t *testing.T, db *pgxpool.Pool, pid int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for {
		var blocked bool
		if err := db.QueryRow(ctx, "SELECT EXISTS(SELECT FROM pg_locks WHERE pid=$1 AND locktype='advisory' AND NOT granted)", pid).Scan(&blocked); err != nil {
			t.Fatal(err)
		}
		if blocked {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("concurrent writer did not reach blacklist lock")
		case <-time.After(10 * time.Millisecond):
		}
	}
}
