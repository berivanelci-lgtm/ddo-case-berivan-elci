package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/auth"
	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/idempotency"
	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/user"
)

type fakeIdemStore struct {
	stored *idempotency.Stored
	err    error
}

func (f *fakeIdemStore) Lookup(_ *http.Request, _ int64, _, _ string) (*idempotency.Stored, error) {
	return f.stored, f.err
}

func idemHandler(calls *int) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		*calls++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":1}`))
	}
}

func idemRequest(t *testing.T, tokens *auth.TokenManager, key, body string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+issueToken(t, tokens, user.RoleOperator, auth.AccessToken))
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	return req
}

func TestIdempotent_NoKeyRunsHandler(t *testing.T) {
	tokens := newTestTokens(t)
	calls := 0
	handler := Authenticate(tokens)(Idempotent(&fakeIdemStore{}, idemHandler(&calls)))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, idemRequest(t, tokens, "", `{}`))

	if calls != 1 || rec.Code != http.StatusCreated {
		t.Fatalf("anahtarsız istek işlenmeli: çağrı %d, kod %d", calls, rec.Code)
	}
}

func TestIdempotent_ReplaysStoredResponse(t *testing.T) {
	tokens := newTestTokens(t)
	calls := 0
	store := &fakeIdemStore{stored: &idempotency.Stored{StatusCode: 201, Body: []byte(`{"id":7}`)}}
	handler := Authenticate(tokens)(Idempotent(store, idemHandler(&calls)))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, idemRequest(t, tokens, "k-1", `{}`))

	if calls != 0 {
		t.Fatal("tekrar istekte işleyici çalışmamalı")
	}
	if rec.Code != http.StatusCreated || rec.Body.String() != `{"id":7}` {
		t.Fatalf("kaydedilmiş yanıt dönmeli: %d %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Idempotent-Replayed") != "true" {
		t.Error("Idempotent-Replayed başlığı true olmalı")
	}
}

func TestIdempotent_ReusedKeyWithDifferentBodyIs422(t *testing.T) {
	tokens := newTestTokens(t)
	calls := 0
	store := &fakeIdemStore{err: idempotency.ErrKeyReused}
	handler := Authenticate(tokens)(Idempotent(store, idemHandler(&calls)))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, idemRequest(t, tokens, "k-1", `{"good":2}`))

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("422 beklenirdi, gelen %d", rec.Code)
	}
	if calls != 0 {
		t.Fatal("yeniden kullanılmış anahtarda işleyici çalışmamalı")
	}
}

func TestIdempotent_TooLongKeyIs400(t *testing.T) {
	tokens := newTestTokens(t)
	calls := 0
	handler := Authenticate(tokens)(Idempotent(&fakeIdemStore{}, idemHandler(&calls)))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, idemRequest(t, tokens, strings.Repeat("a", 129), `{}`))

	if rec.Code != http.StatusBadRequest || calls != 0 {
		t.Fatalf("400 ve işleyici yok beklenirdi: %d, çağrı %d", rec.Code, calls)
	}
}

func TestIdempotent_LookupErrorIs500WithoutLeak(t *testing.T) {
	tokens := newTestTokens(t)
	calls := 0
	store := &fakeIdemStore{err: errors.New("bağlantı koptu: 10.0.0.12")}
	handler := Authenticate(tokens)(Idempotent(store, idemHandler(&calls)))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, idemRequest(t, tokens, "k-1", `{}`))

	if rec.Code != http.StatusInternalServerError || calls != 0 {
		t.Fatalf("500 ve işleyici yok beklenirdi: %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "10.0.0.12") {
		t.Error("iç hata sızmamalı")
	}
}

var _ = context.Background
