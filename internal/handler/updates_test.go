package handler

import (
	"crypto/sha256"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrivateUpdateDelivery(t *testing.T) {
	dir := t.TempDir()
	hash := sha256.Sum256([]byte("synthetic-device-token"))
	mux := http.NewServeMux()
	New(&mobileService{}).RegisterUpdates(mux, hash[:], dir)
	get := func(route string, auth bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", route, nil)
		if auth {
			r.Header.Set("Authorization", "Bearer synthetic-device-token")
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	if get("/mobile/app-update", false).Code != 401 || get("/mobile/app-update", true).Code != 204 {
		t.Fatal("auth/no release")
	}
	release := appRelease{PackageID: "ru.mralexandrov.locations", VersionCode: 3, VersionName: "synthetic", SHA256: strings.Repeat("a", 64), CertificateSHA256: strings.Repeat("b", 64), Bytes: 3, MinSDK: 26}
	data, _ := json.Marshal(release)
	if err := os.WriteFile(filepath.Join(dir, "current.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, release.SHA256+".apk"), []byte("apk"), 0600); err != nil {
		t.Fatal(err)
	}
	if get("/mobile/app-update", true).Code != 200 {
		t.Fatal("manifest unavailable")
	}
	result := get("/mobile/app-update/"+release.SHA256+".apk", true)
	if result.Code != 200 || result.Body.String() != "apk" || result.Header().Get("Content-Type") != "application/vnd.android.package-archive" {
		t.Fatal("APK unavailable")
	}
	if get("/mobile/app-update/"+release.SHA256+".apk", false).Code != 401 || get("/mobile/app-update/current.json", true).Code != 404 {
		t.Fatal("unexpected file exposed")
	}
}
