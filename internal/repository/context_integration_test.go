package repository_test

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"location/internal/handler"
	"location/internal/migrate"
	"location/internal/repository"
	"location/internal/service"
	"location/migrations"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestOptionalContextAcrossPlatformsIsAtomicAndDoesNotHidePoints(t *testing.T) {
	db := isolatedPostgres(t)
	ctx := t.Context()
	if err := migrate.Run(ctx, db, migrations.FS); err != nil {
		t.Fatal(err)
	}
	example, err := os.ReadFile("../../contracts/examples/context-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	repo := repository.NewPostgres(db)
	h := handler.New(service.New(repo, nil, 100))
	mux := http.NewServeMux()
	token := "synthetic-context-token"
	hash := sha256.Sum256([]byte(token))
	h.RegisterMobile(mux, hash[:])
	h.RegisterIOS(mux, hash[:])
	at := time.Date(2026, 11, 24, 12, 1, 0, 0, time.UTC)
	put := func(id, source, c string, want int) {
		t.Helper()
		body := fmt.Sprintf(`{"latitude":10,"longitude":20,"accuracy":5,"recorded_at":%q`, at.Format(time.RFC3339))
		if c != "" {
			body += `,"context":` + c
		}
		body += "}"
		route := "/mobile/locations/"
		if source == "ios" {
			route = "/mobile/ios/locations/"
		}
		req := httptest.NewRequest("PUT", route+id, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != want {
			t.Fatalf("%s: %d %s", source, w.Code, w.Body.String())
		}
	}
	id := uuid.NewString()
	put(id, "android", string(example), 200)
	put(id, "android", string(example), 200)
	var status, device string
	var steps int64
	var hidden bool
	if err := db.QueryRow(ctx, `SELECT context_status,device_id,steps_count,hidden FROM location_context WHERE id=$1`, id).Scan(&status, &device, &steps, &hidden); err != nil || status != "valid" || device != "mobile:android:primary" || steps != 12 || hidden {
		t.Fatalf("projection: %s %s %d %v %v", status, device, steps, hidden, err)
	}
	changed := strings.Replace(string(example), `"count": 12`, `"count": 13`, 1)
	put(id, "android", changed, 409)
	for _, c := range []string{`{"version":99}`, `{"version":1,"steps":{"status":"observed"}}`, `{"version":9223372036854775807}`} {
		id := uuid.NewString()
		put(id, "ios", c, 200)
		if err := db.QueryRow(ctx, `SELECT hidden FROM locations WHERE id=$1`, id).Scan(&hidden); err != nil || hidden {
			t.Fatalf("optional metadata hid location: %v", err)
		}
	}
	ios := strings.Replace(string(example), `"platform": "android"`, `"platform": "ios"`, 1)
	put(uuid.NewString(), "ios", ios, 200)
	legacy := uuid.NewString()
	put(legacy, "ios", "", 200)
	put(legacy, "ios", "", 200)
	if err := db.QueryRow(ctx, `SELECT context_status FROM location_context WHERE id=$1`, legacy).Scan(&status); err != nil || status != "missing" {
		t.Fatal(status, err)
	}
	// A metadata storage failure must roll back the coordinate and prevent an ACK.
	if _, err := db.Exec(ctx, `ALTER TABLE location_measurement_context ADD CONSTRAINT synthetic_rejection CHECK (device_id <> 'mobile:android:primary') NOT VALID`); err != nil {
		t.Fatal(err)
	}
	failed := uuid.NewString()
	put(failed, "android", string(example), 503)
	var n int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM locations WHERE id=$1`, failed).Scan(&n); err != nil || n != 0 {
		t.Fatal("partial commit", n, err)
	}
	var raw json.RawMessage
	if err := db.QueryRow(ctx, `SELECT raw_context FROM location_context WHERE id=$1`, id).Scan(&raw); err != nil || !json.Valid(raw) {
		t.Fatal("raw context lost", err)
	}
}
