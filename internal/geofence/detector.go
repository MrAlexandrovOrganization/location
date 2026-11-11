package geofence

import (
	"math"
	"time"
)

const (
	Freshness = 3 * time.Minute
	StateGap  = 5 * time.Minute
)

type State struct {
	Stable     string
	Candidate  string
	Since      *time.Time
	Last       *time.Time
	LocationID string
}

type Fix struct {
	ID                            string
	Latitude, Longitude, Accuracy float64
	At                            time.Time
}

// Advance requires two separated observations, not a timer without a fresh fix.
// Unknown initialization and resumption after a gap deliberately emit no event.
func Advance(s State, pLat, pLon, radius float64, confirmation time.Duration, f Fix) (State, string, time.Time) {
	if s.Last != nil && !f.At.After(*s.Last) {
		return s, "", time.Time{}
	}
	if s.Last == nil || f.At.Sub(*s.Last) > StateGap {
		s.Stable, s.Candidate, s.Since = "unknown", "", nil
	}
	s.Last, s.LocationID = &f.At, f.ID
	d := distance(pLat, pLon, f.Latitude, f.Longitude)
	next := ""
	if d+f.Accuracy <= radius {
		next = "inside"
	}
	if d-f.Accuracy >= radius+math.Max(50, radius*0.25) {
		next = "outside"
	}
	if next == "" || next == s.Stable {
		s.Candidate, s.Since = "", nil
		return s, "", time.Time{}
	}
	if s.Candidate != next || s.Since == nil {
		s.Candidate, s.Since = next, &f.At
		return s, "", time.Time{}
	}
	if f.At.Sub(*s.Since) < confirmation {
		return s, "", time.Time{}
	}
	previous, since := s.Stable, *s.Since
	s.Stable, s.Candidate, s.Since = next, "", nil
	if previous == "unknown" {
		return s, "", time.Time{}
	}
	transition := "entered"
	if next == "outside" {
		transition = "exited"
	}
	return s, transition, since
}

func distance(lat1, lon1, lat2, lon2 float64) float64 {
	r := math.Pi / 180
	a := math.Pow(math.Sin((lat2-lat1)*r/2), 2) + math.Cos(lat1*r)*math.Cos(lat2*r)*math.Pow(math.Sin((lon2-lon1)*r/2), 2)
	return 6371000 * 2 * math.Asin(math.Sqrt(math.Min(1, math.Max(0, a))))
}
