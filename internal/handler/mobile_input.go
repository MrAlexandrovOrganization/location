package handler

import (
	"bytes"
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

func legacyMobileInput(id string, raw []byte, now time.Time) (model.CreateInput, error) {
	var body struct {
		Latitude   *float64  `json:"latitude"`
		Longitude  *float64  `json:"longitude"`
		Accuracy   *float32  `json:"accuracy"`
		RecordedAt time.Time `json:"recorded_at"`
		Provider   string    `json:"provider"`
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
	return model.CreateInput{ID: id, Latitude: *body.Latitude, Longitude: *body.Longitude, Accuracy: *body.Accuracy,
		RecordedAt: body.RecordedAt, Date: body.RecordedAt.UTC().Format("2006-01-02"), Source: "android", Provider: body.Provider}, nil
}
