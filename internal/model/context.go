package model

import (
	"encoding/json"
	"github.com/google/uuid"
	"strings"
	"time"
)

// MeasurementContext is an observation-only projection. Quality rules never read it.
type MeasurementContext struct {
	DeviceID string
	Status   string
	Version  *int
	Data     json.RawMessage
}

type ContextClient struct {
	InstallationID string `json:"installation_id,omitempty"`
	Platform       string `json:"platform,omitempty"`
	AppVersion     string `json:"app_version,omitempty"`
}
type StepContext struct {
	Status string     `json:"status"`
	Count  *int64     `json:"count,omitempty"`
	From   *time.Time `json:"from,omitempty"`
	To     *time.Time `json:"to,omitempty"`
}
type MovementContext struct {
	Status         string     `json:"status"`
	LastEventAt    *time.Time `json:"last_event_at,omitempty"`
	Event          string     `json:"event,omitempty"`
	ObservingSince *time.Time `json:"observing_since,omitempty"`
}
type CollectionContext struct {
	Mode                string `json:"mode,omitempty"`
	RequestedIntervalMS *int64 `json:"requested_interval_ms,omitempty"`
}
type ContextV1 struct {
	Version    int                        `json:"version"`
	Client     *ContextClient             `json:"client,omitempty"`
	Steps      *StepContext               `json:"steps,omitempty"`
	Movement   *MovementContext           `json:"movement,omitempty"`
	Collection *CollectionContext         `json:"collection,omitempty"`
	Platform   map[string]json.RawMessage `json:"platform,omitempty"`
}

// Device identity comes from the authenticated route, never a client supplied ID.
// Current credential slots allow one Android and one iOS device. Future multi-device
// authentication can supply additional identities without changing context v1.
func ContextDeviceID(source string) string {
	switch source {
	case "android", "ios":
		return "mobile:" + source + ":primary"
	default:
		return "source:" + source
	}
}

func ParseMeasurementContext(payload []byte, source string) *MeasurementContext {
	var envelope map[string]json.RawMessage
	if json.Unmarshal(payload, &envelope) != nil {
		return nil
	}
	raw, ok := envelope["context"]
	if !ok || string(raw) == "null" {
		return nil
	}
	result := &MeasurementContext{DeviceID: ContextDeviceID(source), Status: "invalid"}
	if len(raw) > 8192 {
		result.Status = "too_large"
		return result
	}
	var header struct {
		Version *int `json:"version"`
	}
	if json.Unmarshal(raw, &header) != nil || header.Version == nil {
		return result
	}
	result.Version = header.Version
	if *header.Version != 1 {
		result.Status = "unsupported_version"
		return result
	}
	var c ContextV1
	if json.Unmarshal(raw, &c) != nil {
		return result
	}
	if c.Client != nil {
		if c.Client.Platform != "" && c.Client.Platform != source {
			return result
		}
		if c.Client.InstallationID != "" {
			if _, err := uuid.Parse(c.Client.InstallationID); err != nil {
				return result
			}
		}
		if len(c.Client.AppVersion) > 100 {
			return result
		}
	}
	if c.Steps != nil {
		s := c.Steps
		if !contextStatus(s.Status) {
			return result
		}
		if s.Status == "observed" {
			if s.Count == nil || *s.Count < 0 || !contextTime(s.From) || !contextTime(s.To) || s.From.After(*s.To) {
				return result
			}
		} else if s.Count != nil || s.From != nil || s.To != nil {
			return result
		}
	}
	if c.Movement != nil {
		m := c.Movement
		if !contextStatus(m.Status) || len(m.Event) > 64 {
			return result
		}
		if m.LastEventAt != nil && (!contextTime(m.LastEventAt) || m.Status != "observed" || strings.TrimSpace(m.Event) == "") {
			return result
		}
		if m.Event != "" && m.LastEventAt == nil {
			return result
		}
		if m.ObservingSince != nil && !contextTime(m.ObservingSince) {
			return result
		}
	}
	if c.Collection != nil {
		if c.Collection.Mode != "" && c.Collection.Mode != "normal" && c.Collection.Mode != "stationary" && c.Collection.Mode != "unknown" {
			return result
		}
		if n := c.Collection.RequestedIntervalMS; n != nil && (*n < 0 || *n > 86400000) {
			return result
		}
	}
	data, err := json.Marshal(c)
	if err != nil {
		return result
	}
	result.Status = "valid"
	result.Data = data
	return result
}
func contextStatus(s string) bool {
	switch s {
	case "observed", "unsupported", "permission_denied", "disabled", "unavailable", "unknown":
		return true
	}
	return false
}
func contextTime(t *time.Time) bool { return t != nil && t.Year() >= 2000 && t.Year() <= 2100 }
