package adapter

import (
	"encoding/json"
	"testing"
	"time"
)

func TestTelegramNormalizationKeepsOriginalUpdateAndUsesEditTime(t *testing.T) {
	received := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)
	fields := map[string]any{"update_id": 4, "chat_id": 42, "message_id": 9, "message_date": received.Add(-24 * time.Hour).Unix(), "edit_date": time.Date(2026, 1, 2, 2, 0, 0, 0, time.UTC).Unix(), "timezone_offset_hours": 3, "day_start_hour": 7, "location": map[string]any{"latitude": 10, "longitude": 20, "horizontal_accuracy": 5, "heading": 180}}
	raw, _ := json.Marshal(fields)
	in := Telegram(raw, received)
	if in.InputReason != "" || in.Date != "2026-01-01" || in.Source != "telegram-bot" || in.RecordedAt.Hour() != 2 || string(in.RawPayload) != string(raw) {
		t.Fatal("Telegram normalization failed")
	}
	again := Telegram(raw, received.Add(time.Minute))
	if again.ID != in.ID || again.RawSHA256 != in.RawSHA256 {
		t.Fatal("retry lost identity")
	}
	fields["edit_date"] = time.Date(2026, 1, 2, 5, 0, 0, 0, time.UTC).Unix()
	raw, _ = json.Marshal(fields)
	if Telegram(raw, received).Date != "2026-01-02" {
		t.Fatal("logical day boundary")
	}
}
