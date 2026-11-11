package geofence

import (
	"testing"
	"time"
)

func TestTransitions(t *testing.T) {
	now := time.Now().UTC()
	s := State{Stable: "unknown"}
	fix := func(lon float64, seconds int) (string, time.Time) {
		var event string
		var at time.Time
		s, event, at = Advance(s, 0, 0, 150, time.Minute, Fix{ID: "fix", Longitude: lon, Accuracy: 5, At: now.Add(time.Duration(seconds) * time.Second)})
		return event, at
	}
	if e, _ := fix(0, 0); e != "" {
		t.Fatal(e)
	}
	if e, _ := fix(0, 60); e != "" || s.Stable != "inside" {
		t.Fatalf("baseline: %+v %s", s, e)
	}
	if e, _ := fix(.003, 120); e != "" {
		t.Fatal(e)
	}
	// A second provider's callback at almost the same time does not confirm.
	if e, _ := fix(.003, 121); e != "" {
		t.Fatal(e)
	}
	if e, at := fix(.003, 180); e != "exited" || !at.Equal(now.Add(120*time.Second)) {
		t.Fatalf("exit: %s %v", e, at)
	}
	if e, _ := fix(.003, 240); e != "" {
		t.Fatal("repeated exit")
	}
	fix(0, 300)
	if e, _ := fix(0, 360); e != "entered" {
		t.Fatal(e)
	}
	if e, _ := fix(.003, 100); e != "" || s.Stable != "inside" {
		t.Fatal("out of order changed state")
	}
	fix(.003, 900)
	if e, _ := fix(.003, 960); e != "" || s.Stable != "outside" {
		t.Fatal("gap fabricated an exit")
	}
}
func TestUncertaintyCancelsCandidate(t *testing.T) {
	now := time.Now()
	s := State{Stable: "inside", Last: &now}
	s, _, _ = Advance(s, 0, 0, 150, time.Minute, Fix{Longitude: .003, Accuracy: 5, At: now.Add(time.Minute)})
	s, e, _ := Advance(s, 0, 0, 150, time.Minute, Fix{Longitude: .0015, Accuracy: 50, At: now.Add(2 * time.Minute)})
	if e != "" || s.Candidate != "" || s.Stable != "inside" {
		t.Fatalf("uncertain boundary: %+v", s)
	}
}
