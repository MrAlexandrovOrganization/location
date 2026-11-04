package repository_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"location/internal/migrate"
	"location/internal/model"
	"location/internal/repository"
	"location/internal/service"
	"location/migrations"
)

// TEST_DATABASE_URL must point to a disposable database: this test owns public.
func TestPostgresDurableRetry(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to an isolated PostgreSQL database")
	}
	ctx := context.Background()
	config, err := pgxpool.ParseConfig(url)
	if err != nil || config.ConnConfig.Database != "location_test" {
		t.Fatal("integration test requires a disposable database named location_test")
	}
	db, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public"); err != nil {
		t.Fatal(err)
	}
	// Apply the historical schema first and preserve an existing Telegram point.
	old, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{"001_init.sql", "002_hidden.sql", "003_source.sql"} {
		data, _ := migrations.FS.ReadFile(file)
		if _, err := old.Exec(ctx, string(data)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := old.Exec(ctx, `CREATE TABLE schema_migrations (name TEXT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT now()); INSERT INTO schema_migrations(name) VALUES ('001_init.sql'),('002_hidden.sql'),('003_source.sql'); INSERT INTO locations(id,latitude,longitude,date,source) VALUES ('legacy',1,2,'2026-01-01','telegram-bot')`); err != nil {
		t.Fatal(err)
	}
	if err := old.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := migrate.Check(ctx, db, migrations.FS); err == nil {
		t.Fatal("startup accepted a pending migration")
	}
	if err := migrate.Run(ctx, db, migrations.FS); err != nil {
		t.Fatal(err)
	}
	if err := migrate.Run(ctx, db, migrations.FS); err != nil {
		t.Fatal(err)
	}
	if err := migrate.Check(ctx, db, migrations.FS); err != nil {
		t.Fatal(err)
	}
	repo := repository.NewPostgres(db)
	svc := service.New(repo, nil, 100)
	in := model.CreateInput{ID: "f6fa6818-c1bb-420b-96bd-6c402103b2bb", Latitude: 55, Longitude: 37, Accuracy: 5, Date: "2026-10-01", Source: "android", RecordedAt: time.Date(2026, 10, 1, 12, 0, 0, 123000000, time.UTC)}
	first, err := svc.Save(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	// Simulate the server committing but its response being lost; retry after reconnect.
	for range 3 {
		again, err := svc.Save(ctx, in)
		if err != nil || again.ID != first.ID {
			t.Fatalf("retry: %+v %v", again, err)
		}
	}
	in.Latitude = 56
	if _, err := svc.Save(ctx, in); !errors.Is(err, repository.ErrConflict) {
		t.Fatalf("conflict: %v", err)
	}
	var count int
	if err := db.QueryRow(ctx, "SELECT count(*) FROM locations").Scan(&count); err != nil || count != 2 {
		t.Fatalf("count=%d err=%v", count, err)
	}
	legacy, err := repo.Get(ctx, "legacy")
	if err != nil || legacy.Source != "telegram-bot" {
		t.Fatal("historical data lost", err)
	}
	stored, err := repo.Get(ctx, first.ID)
	if err != nil || stored.Latitude != 55 {
		t.Fatal("retry overwrote stored point", err)
	}
}
