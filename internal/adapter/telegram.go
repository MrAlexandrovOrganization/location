package adapter

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"location/internal/model"
	"math"
	"time"
)

// Telegram owns the Telegram-specific timestamp and logical-day interpretation.
// Frontends forward the original location and message timestamps without deriving a point.
func Telegram(payload json.RawMessage, received time.Time) model.CreateInput {
	sum := sha256.Sum256(payload)
	in := model.CreateInput{Source: "telegram-bot", Provider: "unknown", RecordedAt: received.UTC(), RawPayload: payload, RawSHA256: hex.EncodeToString(sum[:]), InputReason: model.InvalidMeasurement}
	in.ID = uuid.NewSHA1(uuid.NameSpaceURL, payload).String()
	var raw struct {
		UpdateID            int64 `json:"update_id"`
		ChatID              int64 `json:"chat_id"`
		MessageID           int64 `json:"message_id"`
		MessageDate         int64 `json:"message_date"`
		EditDate            int64 `json:"edit_date"`
		TimezoneOffsetHours int   `json:"timezone_offset_hours"`
		DayStartHour        int   `json:"day_start_hour"`
		Location            struct {
			Latitude   *float64 `json:"latitude"`
			Longitude  *float64 `json:"longitude"`
			Accuracy   float32  `json:"horizontal_accuracy"`
			LivePeriod int      `json:"live_period"`
		} `json:"location"`
	}
	if json.Unmarshal(payload, &raw) != nil {
		return finish(in)
	}
	if raw.MessageID > 0 && raw.ChatID != 0 && raw.UpdateID >= 0 {
		in.ID = uuid.NewSHA1(uuid.NameSpaceURL, []byte(fmt.Sprintf("telegram-location:%d:%d:%d", raw.ChatID, raw.MessageID, raw.UpdateID))).String()
	}
	stamp := raw.MessageDate
	if raw.EditDate > stamp {
		stamp = raw.EditDate
	}
	at := time.Unix(stamp, 0).UTC()
	validTime := !at.Before(time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)) && !at.After(received.Add(24*time.Hour))
	if validTime {
		in.RecordedAt = at
	}
	valid := validTime && raw.Location.Latitude != nil && raw.Location.Longitude != nil && raw.Location.Accuracy >= 0 && !math.IsNaN(float64(raw.Location.Accuracy)) && !math.IsInf(float64(raw.Location.Accuracy), 0)
	if raw.Location.Latitude != nil && math.Abs(*raw.Location.Latitude) <= 90 {
		in.Latitude = *raw.Location.Latitude
	} else {
		valid = false
	}
	if raw.Location.Longitude != nil && math.Abs(*raw.Location.Longitude) <= 180 {
		in.Longitude = *raw.Location.Longitude
	} else {
		valid = false
	}
	if raw.TimezoneOffsetHours < -12 || raw.TimezoneOffsetHours > 14 || raw.DayStartHour < 0 || raw.DayStartHour > 23 {
		return finish(in)
	}
	in.Accuracy = raw.Location.Accuracy
	in.LivePeriod = raw.Location.LivePeriod
	in = finish(in)
	in.Date = in.RecordedAt.Add(time.Duration(raw.TimezoneOffsetHours-raw.DayStartHour) * time.Hour).Format("2006-01-02")
	if valid {
		in.InputReason = ""
	}
	return in
}
