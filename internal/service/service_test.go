package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"location/internal/model"
)

type referenceRepository struct {
	prev      *model.Location
	err       error
	before    time.Time
	normal    int
	outliers  int
	reference string
}

func (r *referenceRepository) Save(context.Context, *model.Location) error { r.normal++; return nil }
func (r *referenceRepository) SaveOutlier(_ context.Context, _ *model.Location, reference string) error {
	r.outliers++
	r.reference = reference
	return nil
}
func (r *referenceRepository) GetLatestValid(_ context.Context, _ string, before time.Time) (*model.Location, error) {
	r.before = before
	return r.prev, r.err
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
		{"metro movement", 55.7878375, 37.7827514, false},
		{"impossible jump", 56.167272, 38.255947, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := &referenceRepository{prev: &model.Location{ID: "reference", Latitude: 55.7877374, Longitude: 37.7804614, RecordedAt: when.Add(-30 * time.Second)}}
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
