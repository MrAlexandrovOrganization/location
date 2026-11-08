package repository_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"location/internal/handler"
	"location/internal/migrate"
	"location/internal/repository"
	"location/internal/service"
	"location/migrations"
)

func TestBatchPreservesSingleRequestIdentityAndLostAcknowledgements(t *testing.T) {
	db := isolatedPostgres(t)
	ctx := context.Background()
	if err := migrate.Run(ctx, db, migrations.FS); err != nil {
		t.Fatal(err)
	}
	repo := repository.NewPostgres(db)
	h := handler.New(service.New(repo, nil, 100))
	mux := http.NewServeMux()
	hash := sha256.Sum256([]byte("synthetic-token"))
	h.RegisterMeasurements(mux, hash[:])
	h.RegisterBatches(mux, hash[:])
	id := "e377ab20-b1ee-4ddb-971a-033de66c6fb9"
	second := "914ed933-a5fb-405f-b688-0420324c4ae5"
	raw := "{ \"schema_version\":2, \"latitude\":\"NaN\", \"extras\":{\"synthetic\":\"Тест\"} }\n"
	call := func(method, path string, body []byte) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, bytes.NewReader(body))
		r.Header.Set("Authorization", "Bearer synthetic-token")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	if w := call("PUT", "/mobile/measurements/"+id, []byte(raw)); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	encode := func(first string) []byte {
		b, _ := json.Marshal(map[string]any{"points": []map[string]string{{"id": id, "format": "measurement", "payload": first}, {"id": second, "format": "measurement", "payload": raw}}})
		return b
	}
	// Discard the first batch response, then retry the identical original strings.
	for range 2 {
		w := call("POST", "/mobile/batches", encode(raw))
		if w.Code != 200 || strings.Count(w.Body.String(), `"stored":true`) != 2 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	sum := sha256.Sum256([]byte(raw))
	for _, key := range []string{id, second} {
		p, err := repo.Get(ctx, key)
		if err != nil || p.RawSHA256 != hex.EncodeToString(sum[:]) {
			t.Fatal("retry changed original measurement", err)
		}
	}
	w := call("POST", "/mobile/batches", encode(strings.TrimSpace(raw)))
	if !strings.Contains(w.Body.String(), `"status":409`) {
		t.Fatal("different original bytes accepted", w.Body.String())
	}
}
