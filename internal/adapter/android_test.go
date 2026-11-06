package adapter

import (
	"encoding/json"
	"location/internal/model"
	"testing"
	"time"
)

func TestAndroidKeepsRawDataAndNormalizesOnServer(t *testing.T) {
	when := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	base := map[string]any{"schema_version": 2, "collected_at_ms": when.UnixMilli(), "collected_elapsed_realtime_ns": int64(300 * time.Second), "elapsed_realtime_ns": int64(299 * time.Second), "provider_time_ms": when.Add(4 * time.Minute).UnixMilli(), "provider": "gps", "latitude": 10.0, "longitude": 20.0, "accuracy": 7.0, "installation_id": "e03f3676-049d-4b45-b46c-95501b94aef1", "boot_count": 3, "extras": map[string]any{"synthetic": true}}
	for _, tt := range []struct {
		name   string
		change func(map[string]any)
		want   model.QualityReason
	}{
		{"fresh with shifted provider clock", func(map[string]any) {}, ""},
		{"stale callback", func(m map[string]any) { m["elapsed_realtime_ns"] = int64(179 * time.Second) }, model.StaleMeasurement},
		{"future monotonic clock", func(m map[string]any) { m["elapsed_realtime_ns"] = int64(301 * time.Second) }, model.InvalidMeasurement},
		{"missing accuracy", func(m map[string]any) { m["accuracy"] = nil }, model.InvalidMeasurement},
		{"nonfinite coordinate", func(m map[string]any) { m["latitude"] = "NaN" }, model.InvalidMeasurement},
		{"out of range coordinate", func(m map[string]any) { m["latitude"] = 91 }, model.InvalidMeasurement},
	} {
		t.Run(tt.name, func(t *testing.T) {
			fields := map[string]any{}
			for k, v := range base {
				fields[k] = v
			}
			tt.change(fields)
			raw, _ := json.Marshal(fields)
			in := Android(raw, when.Add(24*time.Hour))
			if in.InputReason != tt.want || string(in.RawPayload) != string(raw) || in.RawSHA256 == "" {
				t.Fatalf("reason=%s want=%s", in.InputReason, tt.want)
			}
			if tt.want == "" && (!in.RecordedAt.Equal(when.Add(-time.Second)) || in.MeasurementKey == "" || in.Provider != "gps") {
				t.Fatal("normalization lost metadata")
			}
			if tt.name == "future monotonic clock" && in.MeasurementKey != "" {
				t.Fatal("invalid clock can poison measurement order")
			}
		})
	}
}
