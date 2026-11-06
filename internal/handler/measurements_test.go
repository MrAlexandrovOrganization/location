package handler

import (
	"crypto/sha256"
	"location/internal/model"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRawEndpointRetainsInvalidMeasurementsButRejectsBadTransport(t *testing.T) {
	hash := sha256.Sum256([]byte("synthetic-device-token"))
	for _, tt := range []struct {
		body, token string
		want        int
	}{
		{`{"schema_version":2,"latitude":"NaN","extras":{"synthetic":true}}`, "synthetic-device-token", 200},
		{`{}`, "wrong", 401},
		{`[]`, "synthetic-device-token", 400},
		{strings.Repeat(" ", 65537) + `{}`, "synthetic-device-token", 400},
	} {
		svc := &mobileService{}
		mux := http.NewServeMux()
		New(svc).RegisterMeasurements(mux, hash[:])
		r := httptest.NewRequest("PUT", "/mobile/measurements/"+pointID, strings.NewReader(tt.body))
		r.Header.Set("Authorization", "Bearer "+tt.token)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != tt.want {
			t.Fatalf("status %d want %d", w.Code, tt.want)
		}
		if tt.want == 200 && (svc.input.InputReason != model.InvalidMeasurement || string(svc.input.RawPayload) != tt.body) {
			t.Fatal("raw rejection was discarded or normalized on client")
		}
	}
}
