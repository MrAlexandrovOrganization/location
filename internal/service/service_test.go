package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"location/internal/model"
)

type referenceRepository struct {
	prev         *model.Location
	err          error
	before       time.Time
	normal       int
	outliers     int
	reference    string
	corroborated bool
	paired       bool
	reason       model.QualityReason
}

func (r *referenceRepository) Save(context.Context, *model.Location) error { r.normal++; return nil }
func (r *referenceRepository) SaveOutlier(_ context.Context, _ *model.Location, reference string, reason model.QualityReason) error {
	r.outliers++
	r.reference = reference
	r.reason = reason
	return nil
}
func (r *referenceRepository) GetReference(_ context.Context, before time.Time, _ string) (*model.Reference, error) {
	r.before = before
	if r.prev == nil {
		return nil, r.err
	}
	return &model.Reference{Point: r.prev, Corroborated: r.corroborated, CorroboratedPair: r.paired}, r.err
}

func TestLargeJumpDoesNotBecomeAReferenceOrEvidence(t *testing.T) {
	when := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	r := &referenceRepository{prev: &model.Location{ID: "home", Latitude: 10.00000, Longitude: 20.00000, Accuracy: 10, RecordedAt: when.Add(-10 * time.Minute)}}
	got, err := New(r, nil, 100).Save(context.Background(), model.CreateInput{Latitude: 10.14, Longitude: 20.00, Accuracy: 5, RecordedAt: when})
	if err != nil || !got.Hidden || got.QualityReason != model.UncertainJump || r.outliers != 0 {
		t.Fatalf("large plausible jump promoted: %+v %v", got, err)
	}
}

func TestCorroboratedImpossibleJumpCanBeBlacklistedImmediately(t *testing.T) {
	when := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	r := &referenceRepository{corroborated: true, prev: &model.Location{ID: "stable", Latitude: 10.00000, Longitude: 20.00000, Accuracy: 10, RecordedAt: when.Add(-30 * time.Second)}}
	got, err := New(r, nil, 100).Save(context.Background(), model.CreateInput{Latitude: 10.19, Longitude: 20.00, Accuracy: 5, RecordedAt: when})
	if err != nil || !got.Hidden || r.reason != model.CorroboratedJump {
		t.Fatalf("missing immediate confirmation: %+v %v", got, err)
	}
}

func TestCoarseFixDoesNotBecomeSpeedEvidence(t *testing.T) {
	r := &referenceRepository{}
	got, err := New(r, nil, 100).Save(context.Background(), model.CreateInput{Accuracy: 1000})
	if err != nil || !got.Hidden || got.QualityReason != model.PoorAccuracy || r.outliers != 0 {
		t.Fatalf("coarse fix: %+v %v", got, err)
	}
}

func TestExtremeJumpRequiresBothSpeedFloorsAndAStablePair(t *testing.T) {
	when := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	for _, tt := range []struct {
		name     string
		latitude float64
		age      time.Duration
		limit    float64
		paired   bool
		want     model.QualityReason
	}{
		{"extreme jump from pair", 10.31, 53 * time.Second, 100, true, model.ExtremeJump},
		{"single reference is insufficient", 10.31, 53 * time.Second, 100, false, model.SpeedOutlier},
		{"ordinary speed violation", 10.10, 53 * time.Second, 100, true, model.SpeedOutlier},
		{"absolute floor protects low configured limit", 10.10, 53 * time.Second, 10, true, model.SpeedOutlier},
		{"relative floor protects high configured limit", 10.31, 53 * time.Second, 250, true, model.SpeedOutlier},
		{"short distance stays ordinary", 10.01, time.Second, 100, true, model.SpeedOutlier},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := &referenceRepository{paired: tt.paired, prev: &model.Location{ID: "reference", Latitude: 10, Longitude: 20, Accuracy: 100, RecordedAt: when.Add(-tt.age)}}
			p, err := New(r, nil, tt.limit).Save(context.Background(), model.CreateInput{Latitude: tt.latitude, Longitude: 20, Accuracy: 7, RecordedAt: when})
			if err != nil || !p.Hidden || p.QualityReason != tt.want || r.reason != tt.want || r.outliers != 1 {
				t.Fatalf("classification: %+v, reason=%s, err=%v", p, r.reason, err)
			}
		})
	}
}
func (*referenceRepository) Get(context.Context, string) (*model.Location, error) { panic("unused") }
func (*referenceRepository) ListByDate(context.Context, string, bool) ([]*model.Location, error) {
	panic("unused")
}
func (*referenceRepository) Delete(context.Context, string) error { panic("unused") }

func TestSpeedDecisions(t *testing.T) {
	when := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	for _, tt := range []struct {
		name       string
		lat, lon   float64
		wantHidden bool
	}{
		{"metro movement", 10.0001, 20.001, false},
		{"impossible jump", 20.0, 30.0, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := &referenceRepository{prev: &model.Location{ID: "reference", Latitude: 10.0, Longitude: 20.0, RecordedAt: when.Add(-30 * time.Second)}}
			got, err := New(r, nil, 100).Save(context.Background(), model.CreateInput{Latitude: tt.lat, Longitude: tt.lon, RecordedAt: when, Date: "2026-01-01"})
			if err != nil || got.Hidden != tt.wantHidden {
				t.Fatalf("point=%+v err=%v", got, err)
			}
			if !r.before.Equal(when) {
				t.Fatal("reference lookup did not bound measurement time")
			}
			if tt.wantHidden {
				if r.outliers != 1 || r.normal != 0 || r.reference != "reference" {
					t.Fatal("rejection was not saved with its reference")
				}
			} else if r.normal != 1 || r.outliers != 0 {
				t.Fatal("normal movement recorded as evidence")
			}
		})
	}
}

func TestReferenceFailureDoesNotBypassFilter(t *testing.T) {
	r := &referenceRepository{err: errors.New("database unavailable")}
	if _, err := New(r, nil, 100).Save(context.Background(), model.CreateInput{}); err == nil {
		t.Fatal("reference failure accepted a point")
	}
	if r.normal != 0 || r.outliers != 0 {
		t.Fatal("point saved without a reference decision")
	}
}
