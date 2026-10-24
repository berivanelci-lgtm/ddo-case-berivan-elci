package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/auth"
	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/user"
)

const middlewareTestSecret = "test-secret-en-az-32-karakter-uzunlukta-xx"

func newTestTokens(t *testing.T) *auth.TokenManager {
	t.Helper()
	m, err := auth.NewTokenManager(middlewareTestSecret, time.Minute, time.Hour)
	if err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}
	return m
}

func issueToken(t *testing.T, m *auth.TokenManager, role user.Role, kind auth.TokenKind) string {
	t.Helper()
	token, err := m.Issue(5, string(role), kind)
	if err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}
	return token
}

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func getWithAuth(handler http.Handler, authorization string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/korumali", nil)
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestAuthenticate_MissingHeader(t *testing.T) {
	handler := Authenticate(newTestTokens(t))(okHandler())

	rec := getWithAuth(handler, "")

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("401 beklenirdi, gelen %d", rec.Code)
	}
	if got := rec.Header().Get("WWW-Authenticate"); got != "Bearer" {
		t.Errorf("WWW-Authenticate: Bearer beklenirdi, gelen %q", got)
	}
	if code := decodeError(t, rec).Error.Code; code != "missing_token" {
		t.Errorf("missing_token beklenirdi, gelen %q", code)
	}
}

func TestAuthenticate_RejectsNonBearerScheme(t *testing.T) {
	handler := Authenticate(newTestTokens(t))(okHandler())

	rec := getWithAuth(handler, "Basic dXNlcjpwYXJvbGE=")

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("401 beklenirdi, gelen %d", rec.Code)
	}
}

func TestAuthenticate_RejectsGarbageToken(t *testing.T) {
	handler := Authenticate(newTestTokens(t))(okHandler())

	rec := getWithAuth(handler, "Bearer abc.def.ghi")

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("401 beklenirdi, gelen %d", rec.Code)
	}
	if code := decodeError(t, rec).Error.Code; code != "invalid_token" {
		t.Errorf("invalid_token beklenirdi, gelen %q", code)
	}
}

func TestAuthenticate_RejectsRefreshToken(t *testing.T) {
	tokens := newTestTokens(t)
	handler := Authenticate(tokens)(okHandler())

	rec := getWithAuth(handler, "Bearer "+issueToken(t, tokens, user.RoleOperator, auth.RefreshToken))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("refresh token ile erişim reddedilmeli, gelen %d", rec.Code)
	}
}

func TestAuthenticate_AcceptsValidAccessToken(t *testing.T) {
	tokens := newTestTokens(t)

	var gotClaims *auth.Claims
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotClaims, _ = ClaimsFrom(r.Context())
		w.WriteHeader(http.StatusOK)
	})
	handler := Authenticate(tokens)(next)

	rec := getWithAuth(handler, "Bearer "+issueToken(t, tokens, user.RoleKaliteMuhendisi, auth.AccessToken))

	if rec.Code != http.StatusOK {
		t.Fatalf("200 beklenirdi, gelen %d", rec.Code)
	}
	if gotClaims == nil || gotClaims.UserID != 5 || gotClaims.Role != string(user.RoleKaliteMuhendisi) {
		t.Fatalf("token bilgisi context'e konmalı, gelen %+v", gotClaims)
	}
}

func TestRequireRoles_AllowsListedRole(t *testing.T) {
	tokens := newTestTokens(t)
	handler := Authenticate(tokens)(RequireRoles(user.RoleHatSefi, user.RoleFabrikaMuduru)(okHandler()))

	rec := getWithAuth(handler, "Bearer "+issueToken(t, tokens, user.RoleFabrikaMuduru, auth.AccessToken))

	if rec.Code != http.StatusOK {
		t.Fatalf("listedeki rol geçmeli, gelen %d", rec.Code)
	}
}

func TestRequireRoles_ForbidsOtherRole(t *testing.T) {
	tokens := newTestTokens(t)
	handler := Authenticate(tokens)(RequireRoles(user.RoleHatSefi)(okHandler()))

	rec := getWithAuth(handler, "Bearer "+issueToken(t, tokens, user.RoleOperator, auth.AccessToken))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("403 beklenirdi, gelen %d", rec.Code)
	}
	if code := decodeError(t, rec).Error.Code; code != "forbidden" {
		t.Errorf("forbidden beklenirdi, gelen %q", code)
	}
}

func TestRequireRoles_WithoutClaimsIsUnauthorized(t *testing.T) {
	handler := RequireRoles(user.RoleHatSefi)(okHandler())

	rec := getWithAuth(handler, "")

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("kimliksiz istek 401 almalı, gelen %d", rec.Code)
	}
}

func TestMeHandler_ReturnsTokenOwner(t *testing.T) {
	tokens := newTestTokens(t)
	handler := Authenticate(tokens)(MeHandler())

	rec := getWithAuth(handler, "Bearer "+issueToken(t, tokens, user.RoleBakim, auth.AccessToken))

	if rec.Code != http.StatusOK {
		t.Fatalf("200 beklenirdi, gelen %d", rec.Code)
	}
	var resp meResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("yanıt çözülemedi: %v", err)
	}
	if resp.ID != 5 || resp.Role != string(user.RoleBakim) {
		t.Fatalf("beklenmeyen yanıt: %+v", resp)
	}
}
