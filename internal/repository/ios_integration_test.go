package repository_test

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"location/internal/handler"
	"location/internal/migrate"
	"location/internal/repository"
	"location/internal/service"
	"location/migrations"
)

func TestIOSIngestionIsolatedFromAndroid(t *testing.T) {
	db := isolatedPostgres(t)
	ctx := t.Context()
	if err := migrate.Run(ctx, db, migrations.FS); err != nil {
		t.Fatal(err)
	}

	const androidToken = "synthetic-android-token"
	const iosToken = "synthetic-ios-token"
	androidHash := sha256.Sum256([]byte(androidToken))
	iosHash := sha256.Sum256([]byte(iosToken))
	repo := repository.NewPostgres(db)
	h := handler.New(service.New(repo, nil, 100))
	mux := http.NewServeMux()
	h.RegisterMobile(mux, androidHash[:])
	h.RegisterIOS(mux, iosHash[:])

	start := time.Now().UTC().Truncate(time.Second).Add(-time.Hour)
	androidID, iosID, laterAndroidID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	laterIOSID := uuid.NewString()
	put := func(path, id, token string, latitude int, offset time.Duration, want int) {
		t.Helper()
		body := fmt.Sprintf(`{"latitude":%d,"longitude":0,"accuracy":5,"recorded_at":%q}`,
			latitude, start.Add(offset).Format(time.RFC3339))
		req := httptest.NewRequest(http.MethodPut, path+id, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		res := httptest.NewRecorder()
		mux.ServeHTTP(res, req)
		if res.Code != want {
			t.Fatalf("status %d, want %d: %s", res.Code, want, res.Body.String())
		}
		if want == http.StatusOK && !strings.Contains(res.Body.String(), fmt.Sprintf(`"id":%q,"stored":true`, id)) {
			t.Fatalf("invalid acknowledgement: %s", res.Body.String())
		}
	}
	const androidPath, iosPath = "/mobile/locations/", "/mobile/ios/locations/"
	put(androidPath, androidID, iosToken, 0, 0, 401)
	put(iosPath, iosID, androidToken, 45, time.Second, 401)
	put(androidPath, androidID, androidToken, 0, 0, 200)
	put(iosPath, iosID, iosToken, 45, time.Second, 200)
	put(iosPath, iosID, iosToken, 45, time.Second, 200) // Lost acknowledgement.
	put(iosPath, iosID, iosToken, 46, time.Second, 409)
	put(androidPath, laterAndroidID, androidToken, 0, 2*time.Second, 200)
	put(iosPath, laterIOSID, iosToken, 46, 45*time.Minute, 200)

	for id, want := range map[string]string{androidID: "android", iosID: "ios", laterAndroidID: "android", laterIOSID: "ios"} {
		point, err := repo.Get(ctx, id)
		if err != nil || point.Source != want || point.Hidden {
			t.Fatalf("stored point: %+v, source should be %s and visible: %v", point, want, err)
		}
	}
	var points, geofencePoints int
	if err := db.QueryRow(ctx, "SELECT (SELECT count(*) FROM locations), (SELECT count(*) FROM geo_pending)").
		Scan(&points, &geofencePoints); err != nil || points != 4 || geofencePoints != 2 {
		t.Fatalf("stored=%d geofence=%d err=%v", points, geofencePoints, err)
	}
}
