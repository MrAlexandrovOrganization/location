package handler

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	"time"

	"location/internal/adapter"
	"location/internal/model"
)

var errMobileInput = errors.New("invalid mobile input")

func measurementInput(id string, raw []byte, now time.Time) (model.CreateInput, error) {
	var fields map[string]json.RawMessage
	if len(raw) > 65536 || json.Unmarshal(raw, &fields) != nil || fields == nil {
		return model.CreateInput{}, errMobileInput
	}
	in := adapter.Android(raw, now)
	in.ID = id
	return in, nil
}

func mobilePointInput(id string, raw []byte, now time.Time, source string) (model.CreateInput, error) {
	var body struct {
		Latitude   *float64        `json:"latitude"`
		Longitude  *float64        `json:"longitude"`
		Accuracy   *float32        `json:"accuracy"`
		RecordedAt time.Time       `json:"recorded_at"`
		Provider   string          `json:"provider"`
		Context    json.RawMessage `json:"context"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if len(raw) > 4096 || decoder.Decode(&body) != nil || decoder.Decode(new(any)) != io.EOF ||
		body.Latitude == nil || body.Longitude == nil || body.Accuracy == nil ||
		math.IsNaN(*body.Latitude) || math.IsInf(*body.Latitude, 0) || math.Abs(*body.Latitude) > 90 ||
		math.IsNaN(*body.Longitude) || math.IsInf(*body.Longitude, 0) || math.Abs(*body.Longitude) > 180 ||
		math.IsNaN(float64(*body.Accuracy)) || math.IsInf(float64(*body.Accuracy), 0) || *body.Accuracy < 0 || !model.ValidProvider(body.Provider) ||
		body.RecordedAt.Before(time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)) || body.RecordedAt.After(now.Add(24*time.Hour)) {
		return model.CreateInput{}, errMobileInput
	}
	in := model.CreateInput{ID: id, Latitude: *body.Latitude, Longitude: *body.Longitude, Accuracy: *body.Accuracy,
		RecordedAt: body.RecordedAt, Date: body.RecordedAt.UTC().Format("2006-01-02"), Source: source, Provider: body.Provider}
	in.Context = model.ParseMeasurementContext(raw, source)
	if in.Context != nil {
		sum := sha256.Sum256(raw)
		in.RawPayload = append([]byte(nil), raw...)
		in.RawSHA256 = hex.EncodeToString(sum[:])
	}
	return in, nil
}
