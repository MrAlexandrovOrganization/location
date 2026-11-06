package repository_test

import (
	"context"
	"testing"
	"time"

	"location/internal/migrate"
	"location/internal/model"
	"location/internal/repository"
	"location/internal/service"
	"location/migrations"
)

func TestLocationQuality(t *testing.T) {
	db := isolatedPostgres(t)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	if err := migrate.Run(ctx, db, migrations.FS); err != nil {
		t.Fatal(err)
	}
	repo := repository.NewPostgres(db)
	svc := service.New(repo, nil, 100)
	when := time.Date(2026, 1, 2, 0, 0, 20, 0, time.UTC)
	reset := func(t *testing.T) {
		t.Helper()
		if _, err := db.Exec(ctx, "TRUNCATE location_outlier_observations, locations, location_blacklist RESTART IDENTITY"); err != nil {
			t.Fatal(err)
		}
	}
	raw := func(t *testing.T, id string, lat, lon float64, at time.Time, provider string, accuracy float32) *model.Location {
		t.Helper()
		p := &model.Location{ID: id, Latitude: lat, Longitude: lon, Accuracy: accuracy, Source: "android", Date: at.Format("2006-01-02"), RecordedAt: at, Provider: provider}
		if err := repo.Save(ctx, p); err != nil {
			t.Fatal(err)
		}
		return p
	}
	in := func(id string, lat, lon float64, at time.Time, provider string) model.CreateInput {
		return model.CreateInput{ID: id, Latitude: lat, Longitude: lon, Accuracy: 5, Source: "android", Date: at.Format("2006-01-02"), RecordedAt: at, Provider: provider}
	}
	count := func(t *testing.T, query string) int {
		t.Helper()
		var n int
		if err := db.QueryRow(ctx, query).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	stable := func(t *testing.T) {
		raw(t, "network-1", 10.00000, 20.00000, when.Add(-90*time.Second), "network", 20)
		raw(t, "network-2", 10.00001, 20.00001, when.Add(-60*time.Second), "network", 20)
		raw(t, "network-3", 10.00000, 20.00000, when.Add(-30*time.Second), "network", 20)
	}

	t.Run("large plausible jump and its follower stay quarantined", func(t *testing.T) {
		reset(t)
		raw(t, "home", 10.00000, 20.00000, when.Add(-10*time.Minute), "unknown", 10)
		for i, at := range []time.Time{when, when.Add(35 * time.Second)} {
			id := []string{"jump", "follower"}[i]
			p, err := svc.Save(ctx, in(id, 10.14, 20.00, at, "unknown"))
			if err != nil || !p.Hidden || p.QualityReason != model.UncertainJump {
				t.Fatalf("quarantine: %+v %v", p, err)
			}
		}
		ref, err := repo.GetReference(ctx, when.Add(time.Minute), "unknown")
		if err != nil || ref.Point.ID != "home" {
			t.Fatal("hidden point poisoned the anchor", err)
		}
		p, err := svc.Save(ctx, in("return-home", 10.00001, 20.00000, when.Add(time.Minute), "unknown"))
		if err != nil || p.Hidden {
			t.Fatal("return to the good anchor hidden", err)
		}
		if count(t, "SELECT count(*) FROM location_outlier_observations") != 0 || count(t, "SELECT count(*) FROM location_blacklist") != 0 {
			t.Fatal("uncertain travel was permanently banned")
		}
	})

	t.Run("reference survives UTC midnight", func(t *testing.T) {
		reset(t)
		raw(t, "yesterday", 10.00000, 20.00000, when.Add(-30*time.Second), "unknown", 10)
		p, err := svc.Save(ctx, in("after-midnight", 10.19, 20.00, when, "unknown"))
		if err != nil || !p.Hidden || p.QualityReason != model.SpeedOutlier {
			t.Fatalf("midnight bypass: %+v %v", p, err)
		}
	})

	t.Run("network corroboration defeats an accepted GPS decoy", func(t *testing.T) {
		reset(t)
		stable(t)
		raw(t, "gps-decoy", 10.14, 20.00, when.Add(-10*time.Second), "gps", 5)
		p, err := svc.Save(ctx, in("gps-outlier", 10.30, 20.00, when, "gps"))
		if err != nil || !p.Hidden || p.QualityReason != model.CorroboratedJump {
			t.Fatalf("corroborated jump: %+v %v", p, err)
		}
		var refID string
		if err := db.QueryRow(ctx, "SELECT reference_id FROM location_outlier_observations WHERE location_id='gps-outlier'").Scan(&refID); err != nil || refID != "network-3" {
			t.Fatal("GPS displaced the network anchor", err)
		}
		if count(t, "SELECT count(*) FROM location_blacklist WHERE enabled AND radius_m=0") != 1 {
			t.Fatal("obvious jump did not get an immediate exact rule")
		}
		p, err = svc.Save(ctx, in("network-home", 10.00001, 20.00000, when.Add(time.Second), "network"))
		if err != nil || p.Hidden {
			t.Fatal("GPS decoy hid a correct network fix", err)
		}
	})

	for _, protection := range []string{"none", "visible visit", "disabled rule"} {
		t.Run("extreme jump from two fixes with "+protection, func(t *testing.T) {
			reset(t)
			raw(t, "pair-1", 10, 20, when.Add(-110*time.Second), "unknown", 25)
			raw(t, "pair-2", 10.00001, 20, when.Add(-53*time.Second), "unknown", 100)
			if protection == "visible visit" {
				raw(t, "earlier-visit", 10.3101, 20, when.Add(-24*time.Hour), "unknown", 20)
			} else if protection == "disabled rule" {
				if _, err := db.Exec(ctx, "INSERT INTO location_blacklist(latitude,longitude,enabled) VALUES(10.31,20,false)"); err != nil {
					t.Fatal(err)
				}
			}
			ref, err := repo.GetReference(ctx, when, "unknown")
			if err != nil || ref == nil || ref.Corroborated || !ref.CorroboratedPair {
				t.Fatal("pair confirmation", ref, err)
			}
			point := in("extreme", 10.31, 20, when, "unknown")
			p, err := svc.Save(ctx, point)
			if err != nil || !p.Hidden || p.QualityReason != model.ExtremeJump {
				t.Fatalf("extreme classification: %+v %v", p, err)
			}
			wantRules := 0
			if protection == "none" {
				wantRules = 1
			}
			if count(t, "SELECT count(*) FROM location_blacklist WHERE enabled AND radius_m=0") != wantRules {
				t.Fatal("immediate rule or protection missing")
			}
			if _, err := svc.Save(ctx, point); err != nil {
				t.Fatal("extreme retry", err)
			}
			if count(t, "SELECT count(*) FROM location_outlier_observations") != 1 {
				t.Fatal("retry duplicated evidence")
			}
			p, err = svc.Save(ctx, in("return-to-pair", 10.00001, 20, when.Add(6*time.Second), "unknown"))
			if err != nil || p.Hidden {
				t.Fatal("extreme jump displaced the correct reference", err)
			}
		})
	}

	for _, support := range []string{"hidden", "poor accuracy", "different provider", "too recent", "too old", "far away", "stale reference"} {
		t.Run("extreme jump needs usable support: "+support, func(t *testing.T) {
			reset(t)
			s := raw(t, "support", 10, 20, when.Add(-110*time.Second), "unknown", 25)
			anchorAt := when.Add(-53 * time.Second)
			switch support {
			case "hidden":
				_, err := db.Exec(ctx, "UPDATE locations SET hidden=true WHERE id=$1", s.ID)
				if err != nil {
					t.Fatal(err)
				}
			case "poor accuracy":
				_, err := db.Exec(ctx, "UPDATE locations SET accuracy=400 WHERE id=$1", s.ID)
				if err != nil {
					t.Fatal(err)
				}
			case "different provider":
				_, err := db.Exec(ctx, "UPDATE locations SET provider='gps' WHERE id=$1", s.ID)
				if err != nil {
					t.Fatal(err)
				}
			case "too recent":
				_, err := db.Exec(ctx, "UPDATE locations SET recorded_at=$2 WHERE id=$1", s.ID, anchorAt.Add(-10*time.Second))
				if err != nil {
					t.Fatal(err)
				}
			case "too old":
				_, err := db.Exec(ctx, "UPDATE locations SET recorded_at=$2 WHERE id=$1", s.ID, anchorAt.Add(-4*time.Minute))
				if err != nil {
					t.Fatal(err)
				}
			case "far away":
				_, err := db.Exec(ctx, "UPDATE locations SET latitude=10.01 WHERE id=$1", s.ID)
				if err != nil {
					t.Fatal(err)
				}
			case "stale reference":
				anchorAt = when.Add(-130 * time.Second)
				_, err := db.Exec(ctx, "UPDATE locations SET recorded_at=$2 WHERE id=$1", s.ID, anchorAt.Add(-time.Minute))
				if err != nil {
					t.Fatal(err)
				}
			}
			raw(t, "anchor", 10, 20, anchorAt, "unknown", 100)
			ref, err := repo.GetReference(ctx, when, "unknown")
			if err != nil || ref == nil || ref.CorroboratedPair {
				t.Fatal("unusable support confirmed a pair", ref, err)
			}
			p, err := svc.Save(ctx, in("unconfirmed", 10.61, 20, when, "unknown"))
			if err != nil || !p.Hidden || p.QualityReason != model.SpeedOutlier {
				t.Fatalf("unconfirmed extreme jump: %+v %v", p, err)
			}
			if count(t, "SELECT count(*) FROM location_blacklist") != 0 {
				t.Fatal("unconfirmed point permanently banned")
			}
		})
	}

	t.Run("lost pair confirmation rolls back the immediate rule", func(t *testing.T) {
		reset(t)
		raw(t, "support", 10, 20, when.Add(-110*time.Second), "unknown", 25)
		raw(t, "anchor", 10, 20, when.Add(-53*time.Second), "unknown", 100)
		ref, err := repo.GetReference(ctx, when, "unknown")
		if err != nil || ref == nil || !ref.CorroboratedPair {
			t.Fatal("missing initial pair", err)
		}
		if _, err := db.Exec(ctx, "UPDATE locations SET hidden=true WHERE id='support'"); err != nil {
			t.Fatal(err)
		}
		p := &model.Location{ID: "lost-pair", Latitude: 10.31, Longitude: 20, Accuracy: 7, Source: "android", Date: when.Format("2006-01-02"), RecordedAt: when, Hidden: true}
		if err := repo.SaveOutlier(ctx, p, ref.Point.ID, model.ExtremeJump); err == nil {
			t.Fatal("lost confirmation accepted")
		}
		if count(t, "SELECT count(*) FROM locations WHERE id='lost-pair'") != 0 || count(t, "SELECT count(*) FROM location_outlier_observations") != 0 || count(t, "SELECT count(*) FROM location_blacklist") != 0 {
			t.Fatal("stale confirmation left partial data")
		}
	})

	for _, hasVisit := range []bool{false, true} {
		name := "immediate promotion respects manual opt-out"
		if hasVisit {
			name = "immediate promotion protects earlier real visits"
		}
		t.Run(name, func(t *testing.T) {
			reset(t)
			stable(t)
			if hasVisit {
				raw(t, "real-visit", 10.3001, 20.00, when.Add(-24*time.Hour), "network", 20)
			} else {
				if _, err := db.Exec(ctx, "INSERT INTO location_blacklist(latitude,longitude,enabled) VALUES(10.30,20.00,false)"); err != nil {
					t.Fatal(err)
				}
			}
			p, err := svc.Save(ctx, in("protected-outlier", 10.30, 20.00, when, "gps"))
			if err != nil || !p.Hidden {
				t.Fatal("outlier not hidden", err)
			}
			if count(t, "SELECT count(*) FROM location_blacklist WHERE enabled") != 0 {
				t.Fatal("manual exception or real visit was permanently banned")
			}
		})
	}

	t.Run("fresh network permits a real journey after a GPS gap", func(t *testing.T) {
		reset(t)
		raw(t, "departure", 10.00000, 20.00000, when.Add(-10*time.Minute), "gps", 10)
		p, err := svc.Save(ctx, in("arrive-network", 10.14, 20.00, when, "network"))
		if err != nil || p.Hidden {
			t.Fatal("network reacquisition hidden", err)
		}
		p, err = svc.Save(ctx, in("arrive-gps", 10.1401, 20.00, when.Add(30*time.Second), "gps"))
		if err != nil || p.Hidden {
			t.Fatal("corroborated arrival hidden", err)
		}
	})

	t.Run("coarse fix is retained without becoming a reference", func(t *testing.T) {
		reset(t)
		raw(t, "good", 10.00000, 20.00000, when.Add(-30*time.Second), "network", 20)
		coarse := in("coarse", 10.14, 20.00, when, "network")
		coarse.Accuracy = 2000
		p, err := svc.Save(ctx, coarse)
		if err != nil || !p.Hidden || p.QualityReason != model.PoorAccuracy {
			t.Fatal("coarse point discarded or promoted", err)
		}
		ref, err := repo.GetReference(ctx, when.Add(time.Second), "network")
		if err != nil || ref.Point.ID != "good" {
			t.Fatal("coarse point became a reference", err)
		}
	})

	t.Run("provider metadata does not break an old queue retry", func(t *testing.T) {
		reset(t)
		in := in("old-queue", 10.00000, 20.00000, when, "unknown")
		if _, err := svc.Save(ctx, in); err != nil {
			t.Fatal(err)
		}
		in.Provider = "gps"
		if _, err := svc.Save(ctx, in); err != nil {
			t.Fatal("optional metadata caused a durable conflict", err)
		}
		if count(t, "SELECT count(*) FROM locations") != 1 {
			t.Fatal("retry duplicated a point")
		}
	})

	t.Run("reference change rolls back a stale classification", func(t *testing.T) {
		reset(t)
		raw(t, "old-reference", 10.00000, 20.00000, when.Add(-30*time.Second), "unknown", 10)
		raw(t, "new-reference", 10.00001, 20.00000, when.Add(-time.Second), "unknown", 10)
		p := &model.Location{ID: "stale-decision", Latitude: 10.30, Longitude: 20.00, Accuracy: 5, Source: "android", Date: when.Format("2006-01-02"), RecordedAt: when, Hidden: true}
		if err := repo.SaveOutlier(ctx, p, "old-reference", model.SpeedOutlier); err == nil {
			t.Fatal("stale reference accepted")
		}
		if count(t, "SELECT count(*) FROM locations WHERE id='stale-decision'") != 0 || count(t, "SELECT count(*) FROM location_outlier_observations") != 0 {
			t.Fatal("stale decision left partial data")
		}
	})
}
