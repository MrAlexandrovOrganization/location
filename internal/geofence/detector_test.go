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

func TestEntryAfterGapRequiresNewConfirmation(t *testing.T) {
	start := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	s := State{Stable: "unknown"}
	for _, step := range []struct {
		seconds   int
		longitude float64
		stable    string
		candidate string
		event     string
	}{
		{0, .003, "unknown", "outside", ""},
		{60, .003, "outside", "", ""},
		{120, 0, "outside", "inside", ""},
		{900, 0, "outside", "inside", ""},
		{929, 0, "outside", "inside", ""},
		{960, 0, "inside", "", "entered"},
		{1020, 0, "inside", "", ""},
	} {
		fix := Fix{Longitude: step.longitude, Accuracy: 5, At: start.Add(time.Duration(step.seconds) * time.Second)}
		next, event, occurredAt := Advance(s, 0, 0, 100, time.Minute, fix)
		if next.Stable != step.stable || next.Candidate != step.candidate || event != step.event {
			t.Fatalf("at %ds: state=%q candidate=%q event=%q", step.seconds, next.Stable, next.Candidate, event)
		}
		if event == "entered" && !occurredAt.Equal(start.Add(900*time.Second)) {
			t.Fatalf("entry started at %v, want first fix after the gap", occurredAt)
		}
		s = next
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
