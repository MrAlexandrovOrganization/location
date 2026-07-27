package model

import "time"

type Location struct {
	ID         string    `json:"id"`
	Latitude   float64   `json:"latitude"`
	Longitude  float64   `json:"longitude"`
	Accuracy   float32   `json:"accuracy"`
	LivePeriod int       `json:"live_period"`
	Date       string    `json:"date"`
	RecordedAt time.Time `json:"recorded_at"`
}

type CreateInput struct {
	Latitude   float64
	Longitude  float64
	Accuracy   float32
	LivePeriod int
	Date       string
	RecordedAt time.Time
}
