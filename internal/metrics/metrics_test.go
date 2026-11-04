package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMobileMetricsUseRouteInsteadOfPointIdentity(t *testing.T) {
	export, shutdown, err := Init()
	if err != nil {
		t.Fatal(err)
	}
	defer shutdown()
	m, err := New()
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /mobile/locations/{id}", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	for _, id := range []string{"private-first-id", "private-second-id"} {
		m.Middleware(mux).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("PUT", "/mobile/locations/"+id, nil))
	}
	res := httptest.NewRecorder()
	export.ServeHTTP(res, httptest.NewRequest("GET", "/metrics", nil))
	body := res.Body.String()
	if strings.Contains(body, "private-first-id") || strings.Contains(body, "private-second-id") || !strings.Contains(body, `path="/mobile/locations/{id}"`) {
		t.Fatal("point identity leaked into metrics")
	}
}
