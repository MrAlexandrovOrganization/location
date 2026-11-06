package adapter

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"strconv"
	"time"

	"github.com/google/uuid"
	"location/internal/model"
)

// Android converts transport measurements to the common service input. Raw data,
// including unknown fields and invalid values, is always retained separately.
func Android(payload json.RawMessage, received time.Time) model.CreateInput {
	sum := sha256.Sum256(payload)
	in := model.CreateInput{Source: "android", Provider: "unknown", RecordedAt: received.UTC(), RawPayload: payload, RawSHA256: hex.EncodeToString(sum[:]), InputReason: model.InvalidMeasurement}
	var fields map[string]json.RawMessage
	if json.Unmarshal(payload, &fields) != nil {
		return finish(in)
	}
	number := func(key string) (float64, bool) {
		var n json.Number
		if json.Unmarshal(fields[key], &n) != nil {
			return 0, false
		}
		v, err := n.Float64()
		return v, err == nil && !math.IsNaN(v) && !math.IsInf(v, 0)
	}
	integer := func(key string) (int64, bool) {
		var n json.Number
		if json.Unmarshal(fields[key], &n) != nil {
			return 0, false
		}
		v, err := n.Int64()
		return v, err == nil
	}
	str := func(key string) string { var s string; _ = json.Unmarshal(fields[key], &s); return s }
	schema, ok := integer("schema_version")
	if !ok || schema != 2 {
		return finish(in)
	}
	collected, validCollected := integer("collected_at_ms")
	if validCollected {
		at := time.UnixMilli(collected)
		validCollected = !at.Before(time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)) && !at.After(received.Add(24*time.Hour))
		if validCollected {
			in.RecordedAt = at.UTC()
		}
	}
	elapsed, validElapsed := integer("elapsed_realtime_ns")
	nowElapsed, validNow := integer("collected_elapsed_realtime_ns")
	validClock := validCollected && validElapsed && validNow && elapsed >= 0 && nowElapsed >= elapsed
	var age time.Duration
	if validClock {
		age = time.Duration(nowElapsed - elapsed)
		in.RecordedAt = in.RecordedAt.Add(-age)
	}
	lat, validLat := number("latitude")
	lon, validLon := number("longitude")
	accuracy, validAccuracy := number("accuracy")
	if validLat && math.Abs(lat) <= 90 {
		in.Latitude = lat
	} else {
		validLat = false
	}
	if validLon && math.Abs(lon) <= 180 {
		in.Longitude = lon
	} else {
		validLon = false
	}
	if validAccuracy && accuracy >= 0 && accuracy <= math.MaxFloat32 {
		in.Accuracy = float32(accuracy)
	} else {
		validAccuracy = false
	}
	provider := str("provider")
	if model.ValidProvider(provider) && provider != "" {
		in.Provider = provider
	}
	installation, err := uuid.Parse(str("installation_id"))
	session, sessionErr := uuid.Parse(str("collection_session_id"))
	if err == nil && validClock && len(provider) <= 128 {
		boot, bootOK := integer("boot_count")
		epoch := ""
		if bootOK && boot >= 0 {
			epoch = "boot:" + strconv.FormatInt(boot, 10)
		} else if sessionErr == nil {
			epoch = "session:" + session.String()
		}
		if epoch != "" {
			in.MeasurementKey = installation.String() + ":" + epoch + ":" + provider
			in.MeasurementElapsedNS = elapsed
		}
	}
	if validClock && validLat && validLon && validAccuracy && !in.RecordedAt.Before(time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)) {
		in.InputReason = ""
		if age > 2*time.Minute {
			in.InputReason = model.StaleMeasurement
		}
	}
	return finish(in)
}

func finish(in model.CreateInput) model.CreateInput {
	in.Date = in.RecordedAt.UTC().Format("2006-01-02")
	return in
}
