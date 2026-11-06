package handler

import (
	"context"
	"crypto/sha256"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"location/internal/model"
	"location/internal/repository"
)

type mobileService struct {
	input model.CreateInput
	err   error
	calls int
}

func (s *mobileService) Save(_ context.Context, in model.CreateInput) (*model.Location, error) {
	s.input = in
	s.calls++
	return &model.Location{ID: in.ID}, s.err
}
func (*mobileService) Get(context.Context, string) (*model.Location, error) { panic("unused") }
func (*mobileService) ListByDate(context.Context, string, bool) ([]*model.Location, error) {
	panic("unused")
}
func (*mobileService) Delete(context.Context, string) error { panic("unused") }

const pointID = "9ac2c2ec-a40a-4949-8d5f-fbd2e6fd9895"
const validPoint = `{"latitude":0,"longitude":0,"accuracy":4.5,"recorded_at":"2026-10-01T10:11:12.123Z"}`

func TestMobileIngestion(t *testing.T) {
	hash := sha256.Sum256([]byte("synthetic-device-token"))
	for _, tt := range []struct {
		name, body, token, id string
		disabled              bool
		err                   error
		want                  int
	}{
		{name: "commit acknowledgement", body: validPoint, token: "synthetic-device-token", want: 200},
		{name: "disabled fails closed", body: validPoint, disabled: true, want: 503},
		{name: "no credential", body: validPoint, want: 401},
		{name: "wrong credential", body: validPoint, token: "wrong", want: 401},
		{name: "missing coordinates", body: `{"recorded_at":"2026-10-01T10:11:12Z"}`, token: "synthetic-device-token", want: 400},
		{name: "invalid range", body: strings.Replace(validPoint, `"latitude":0`, `"latitude":91`, 1), token: "synthetic-device-token", want: 400},
		{name: "negative accuracy", body: strings.Replace(validPoint, `4.5`, `-1`, 1), token: "synthetic-device-token", want: 400},
		{name: "provider metadata", body: strings.Replace(validPoint, `"latitude"`, `"provider":"network","latitude"`, 1), token: "synthetic-device-token", want: 200},
		{name: "invalid provider", body: strings.Replace(validPoint, `"latitude"`, `"provider":"custom","latitude"`, 1), token: "synthetic-device-token", want: 400},
		{name: "unknown fields", body: strings.Replace(validPoint, `"latitude"`, `"source":"telegram-bot","latitude"`, 1), token: "synthetic-device-token", want: 400},
		{name: "trailing JSON", body: validPoint + `{}`, token: "synthetic-device-token", want: 400},
		{name: "huge input", body: strings.Repeat(" ", 4097) + validPoint, token: "synthetic-device-token", want: 400},
		{name: "invalid id", body: validPoint, id: "bad", token: "synthetic-device-token", want: 400},
		{name: "storage failure", body: validPoint, token: "synthetic-device-token", err: errors.New("database unavailable"), want: 503},
		{name: "conflicting data", body: validPoint, token: "synthetic-device-token", err: repository.ErrConflict, want: 409},
	} {
		t.Run(tt.name, func(t *testing.T) {
			svc := &mobileService{err: tt.err}
			mux := http.NewServeMux()
			var credential []byte
			if !tt.disabled {
				credential = hash[:]
			}
			New(svc).RegisterMobile(mux, credential)
			id := tt.id
			if id == "" {
				id = pointID
			}
			req := httptest.NewRequest("PUT", "/mobile/locations/"+id, strings.NewReader(tt.body))
			if tt.token != "" {
				req.Header.Set("Authorization", "Bearer "+tt.token)
			}
			res := httptest.NewRecorder()
			mux.ServeHTTP(res, req)
			if res.Code != tt.want {
				t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
			}
			if res.Code == 200 && (!strings.Contains(res.Body.String(), `"stored":true`) || svc.input.Source != "android" || svc.input.ID != pointID || svc.input.Date != "2026-10-01") {
				t.Fatalf("invalid acknowledgement/input: %s %+v", res.Body.String(), svc.input)
			}
			if res.Code == 400 || res.Code == 401 {
				if svc.calls != 0 {
					t.Fatal("invalid request reached storage")
				}
			}
		})
	}
}
