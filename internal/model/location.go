package model

import "time"

type Location struct {
	ID            string        `json:"id"`
	Latitude      float64       `json:"latitude"`
	Longitude     float64       `json:"longitude"`
	Accuracy      float32       `json:"accuracy"`
	LivePeriod    int           `json:"live_period"`
	Date          string        `json:"date"`
	Source        string        `json:"source"`
	Hidden        bool          `json:"hidden"`
	RecordedAt    time.Time     `json:"recorded_at"`
	Provider      string        `json:"-"`
	QualityReason QualityReason `json:"-"`
}

type QualityReason string

const (
	SpeedOutlier     QualityReason = "speed"
	CorroboratedJump QualityReason = "corroborated_jump"
	UncertainJump    QualityReason = "uncertain_jump"
	PoorAccuracy     QualityReason = "poor_accuracy"
)

type Reference struct {
	Point        *Location
	Corroborated bool
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
	ID         string
	Latitude   float64
	Longitude  float64
	Accuracy   float32
	LivePeriod int
	Date       string
	Source     string
	RecordedAt time.Time
	Provider   string
}
