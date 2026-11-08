package repository_test

import (
	"context"
	_ "embed"
	"testing"
	"time"

	"location/internal/migrate"
	"location/migrations"
)

//go:embed testdata/blacklist_scale.sql
var blacklistScaleSQL string

//go:embed testdata/incremental_blacklist_checks.sql
var incrementalBlacklistChecksSQL string

func TestRuleBackfillUsesOnlyChangedRulesAtHistoryScale(t *testing.T) {
	db := isolatedPostgres(t)
	ctx := context.Background()
	if err := migrate.Run(ctx, db, migrations.FS); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, blacklistScaleSQL); err != nil {
		t.Fatal(err)
	}
	// A deterministic matcher guard catches the former full-history/all-rules
	// algorithm, without relying on a timing comparison between CI machines.
	bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	start := time.Now()
	if _, err := db.Exec(bounded, incrementalBlacklistChecksSQL); err != nil {
		t.Fatal(err)
	}
	t.Logf("backfilled changed exact/radius rules against 32000 points and 1300 existing rules in %s", time.Since(start))
}
