package model

import (
	"encoding/json"
	"os"
	"testing"
)

func TestContextV1OptionalAndInvalidMetadata(t *testing.T) {
	example, err := os.ReadFile("../../contracts/examples/context-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	wrap := func(c string) []byte { return []byte(`{"context":` + c + `}`) }
	for _, tt := range []struct{ context, source, status string }{
		{string(example), "android", "valid"},
		{`{"version":1,"client":{"platform":"ios"},"steps":{"status":"permission_denied"}}`, "ios", "valid"},
		{`{"version":1,"steps":{"status":"observed","count":0,"from":"2026-11-24T12:00:00Z","to":"2026-11-24T12:01:00Z"}}`, "android", "valid"},
		{`{"version":2}`, "android", "unsupported_version"},
		{`{"version":9223372036854775807}`, "android", "unsupported_version"},
		{`true`, "android", "invalid"},
		{`{"version":1,"client":{"platform":"ios"}}`, "android", "invalid"},
		{`{"version":1,"steps":{"status":"observed","count":0}}`, "android", "invalid"},
		{`{"version":1,"steps":{"status":"permission_denied","count":0}}`, "android", "invalid"},
		{`{"version":1,"steps":{"status":"observed","count":-1,"from":"2026-11-24T12:00:00Z","to":"2026-11-24T12:01:00Z"}}`, "android", "invalid"},
		{`{"version":1,"movement":{"status":"observed","event":"step"}}`, "android", "invalid"},
	} {
		t.Run(tt.status+tt.context, func(t *testing.T) {
			c := ParseMeasurementContext(wrap(tt.context), tt.source)
			if c == nil || c.Status != tt.status || c.DeviceID != "mobile:"+tt.source+":primary" {
				t.Fatalf("unexpected context: %+v", c)
			}
			if tt.status == "valid" && !json.Valid(c.Data) {
				t.Fatal("projection lost")
			}
		})
	}
	if ParseMeasurementContext([]byte(`{}`), "android") != nil || ParseMeasurementContext(wrap("null"), "android") != nil {
		t.Fatal("missing context must be optional")
	}
}
