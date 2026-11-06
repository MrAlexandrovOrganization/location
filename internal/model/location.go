package model

import (
	"encoding/json"
	"time"
)

type Location struct {
	ID                   string          `json:"id"`
	Latitude             float64         `json:"latitude"`
	Longitude            float64         `json:"longitude"`
	Accuracy             float32         `json:"accuracy"`
	LivePeriod           int             `json:"live_period"`
	Date                 string          `json:"date"`
	Source               string          `json:"source"`
	Hidden               bool            `json:"hidden"`
	RecordedAt           time.Time       `json:"recorded_at"`
	Provider             string          `json:"-"`
	QualityReason        QualityReason   `json:"-"`
	RawPayload           json.RawMessage `json:"-"`
	RawSHA256            string          `json:"-"`
	MeasurementKey       string          `json:"-"`
	MeasurementElapsedNS int64           `json:"-"`
}

type QualityReason string

const (
	SpeedOutlier          QualityReason = "speed"
	CorroboratedJump      QualityReason = "corroborated_jump"
	ExtremeJump           QualityReason = "extreme_jump"
	UncertainJump         QualityReason = "uncertain_jump"
	PoorAccuracy          QualityReason = "poor_accuracy"
	InvalidMeasurement    QualityReason = "invalid_measurement"
	StaleMeasurement      QualityReason = "stale_measurement"
	DuplicateMeasurement  QualityReason = "duplicate_measurement"
	OutOfOrderMeasurement QualityReason = "out_of_order_measurement"
)

type Reference struct {
	Point        *Location
	Corroborated bool
	// CorroboratedPair has one earlier supporting fix instead of two.
	// It only permits immediate rules for extreme speeds, not ordinary outliers.
	CorroboratedPair bool
}

func ValidProvider(provider string) bool {
	switch provider {
	case "", "unknown", "gps", "network", "passive", "fused":
		return true
	default:
		return false
	}
}

type CreateInput struct {
	// ID stays stable across delivery retries from a durable client.
	ID                   string
	Latitude             float64
	Longitude            float64
	Accuracy             float32
	LivePeriod           int
	Date                 string
	Source               string
	RecordedAt           time.Time
	Provider             string
	RawPayload           json.RawMessage
	RawSHA256            string
	MeasurementKey       string
	MeasurementElapsedNS int64
	InputReason          QualityReason
}
