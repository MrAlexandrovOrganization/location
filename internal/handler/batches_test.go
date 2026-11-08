package handler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"location/internal/model"
	"location/internal/repository"
)

type batchService struct {
	mobileService
	inputs []model.CreateInput
	save   func(context.Context, model.CreateInput) error
}

func (s *batchService) Save(ctx context.Context, in model.CreateInput) (*model.Location, error) {
	s.inputs = append(s.inputs, in)
	if s.save != nil {
		if err := s.save(ctx, in); err != nil {
			return nil, err
		}
	}
	return &model.Location{ID: in.ID}, nil
}
func batchRequest(mux http.Handler, points []mobileBatchPoint, token string) *httptest.ResponseRecorder {
	body, _ := json.Marshal(struct {
		Points []mobileBatchPoint `json:"points"`
	}{points})
	r := httptest.NewRequest("POST", "/mobile/batches", bytes.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	return w
}
func TestMobileBatches(t *testing.T) {
	hash := sha256.Sum256([]byte("synthetic-device-token"))
	ids := []string{pointID, "ac2e8af2-42fa-45e5-b66b-eaa125e077d8", "04a1eb4c-ab2b-4ea9-8c10-fef950e25e6d"}
	for _, badStatus := range []int{400, 409, 503} {
		t.Run(fmt.Sprint(badStatus), func(t *testing.T) {
			s := &batchService{}
			s.save = func(_ context.Context, in model.CreateInput) error {
				if in.ID == ids[1] {
					if badStatus == 409 {
						return repository.ErrConflict
					}
					if badStatus == 503 {
						return errors.New("synthetic-private-database-error")
					}
				}
				return nil
			}
			mux := http.NewServeMux()
			New(s).RegisterBatches(mux, hash[:])
			points := []mobileBatchPoint{{ids[0], "measurement", `{ "schema_version":2, "latitude":"NaN" }`}, {ids[1], "location", validPoint}, {ids[2], "location", validPoint}}
			if badStatus == 400 {
				points[1].Payload = `[]`
			}
			w := batchRequest(mux, points, "synthetic-device-token")
			var response struct {
				Results []mobileBatchResult `json:"results"`
			}
			if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &response) != nil {
				t.Fatal(w.Code, w.Body.String())
			}
			want := 3
			if badStatus == 503 {
				want = 2
			}
			if len(response.Results) != want || !response.Results[0].Stored || response.Results[1].Stored || response.Results[1].Status != badStatus {
				t.Fatal(response)
			}
			if string(s.inputs[0].RawPayload) != points[0].Payload {
				t.Fatal("original raw bytes changed")
			}
			if strings.Contains(w.Body.String(), "private-database") {
				t.Fatal("private error escaped")
			}
		})
	}
	t.Run("validate whole envelope before writes", func(t *testing.T) {
		for _, points := range [][]mobileBatchPoint{
			nil, {{ids[0], "measurement", `{}`}, {ids[0], "measurement", `{}`}},
			{{ids[0], "measurement", `{}`}, {"bad", "measurement", `{}`}},
			{{ids[0], "other", `{}`}}, make([]mobileBatchPoint, 101),
		} {
			s := &batchService{}
			mux := http.NewServeMux()
			New(s).RegisterBatches(mux, hash[:])
			w := batchRequest(mux, points, "synthetic-device-token")
			if w.Code != 400 || len(s.inputs) != 0 {
				t.Fatal(w.Code, len(s.inputs))
			}
		}
	})
	t.Run("authentication and body limits", func(t *testing.T) {
		s := &batchService{}
		mux := http.NewServeMux()
		New(s).RegisterBatches(mux, hash[:])
		if w := batchRequest(mux, []mobileBatchPoint{{ids[0], "measurement", `{}`}}, "wrong"); w.Code != 401 {
			t.Fatal(w.Code)
		}
		r := httptest.NewRequest("POST", "/mobile/batches", strings.NewReader(strings.Repeat(" ", mobileBatchMaxBytes+1)+`{}`))
		r.Header.Set("Authorization", "Bearer synthetic-device-token")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != 400 || len(s.inputs) != 0 {
			t.Fatal(w.Code, len(s.inputs))
		}
	})
	t.Run("one batch in flight", func(t *testing.T) {
		entered, release := make(chan struct{}), make(chan struct{})
		s := &batchService{save: func(context.Context, model.CreateInput) error { close(entered); <-release; return nil }}
		mux := http.NewServeMux()
		New(s).RegisterBatches(mux, hash[:])
		done := make(chan int, 1)
		go func() {
			done <- batchRequest(mux, []mobileBatchPoint{{ids[0], "measurement", `{}`}}, "synthetic-device-token").Code
		}()
		<-entered
		w := batchRequest(mux, []mobileBatchPoint{{ids[2], "measurement", `{}`}}, "synthetic-device-token")
		close(release)
		if w.Code != 503 || <-done != 200 {
			t.Fatal("overlapping batch not limited")
		}
	})
}
