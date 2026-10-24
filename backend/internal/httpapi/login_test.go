package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/auth"
	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/user"
)

// fakeAuthenticator, gerçek servis yerine hazır bir sonuç döner.
type fakeAuthenticator struct {
	user user.User
	pair auth.TokenPair
	err  error
}

func (f fakeAuthenticator) Login(context.Context, string, string) (user.User, auth.TokenPair, error) {
	return f.user, f.pair, f.err
}

type errorEnvelope struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func postLogin(handler http.HandlerFunc, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler(rec, req)
	return rec
}

func decodeError(t *testing.T, rec *httptest.ResponseRecorder) errorEnvelope {
	t.Helper()
	var envelope errorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("hata yanıtı çözülemedi: %v (gövde: %s)", err, rec.Body.String())
	}
	return envelope
}

func TestLoginHandler_Success(t *testing.T) {
	handler := LoginHandler(fakeAuthenticator{
		user: user.User{ID: 7, Email: "hatsefi@hatkontrol.test", FullName: "Demo Hat Şefi", Role: user.RoleHatSefi, PasswordHash: "gizli-ozet"},
		pair: auth.TokenPair{AccessToken: "access-x", RefreshToken: "refresh-y"},
	})

	rec := postLogin(handler, `{"email":"hatsefi@hatkontrol.test","password":"parola"}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("200 beklenirdi, gelen %d", rec.Code)
	}
	if contentType := rec.Header().Get("Content-Type"); contentType != "application/json" {
		t.Errorf("Content-Type application/json olmalı, gelen %q", contentType)
	}

	var resp loginResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("yanıt çözülemedi: %v", err)
	}
	if resp.AccessToken != "access-x" || resp.RefreshToken != "refresh-y" || resp.TokenType != "Bearer" {
		t.Errorf("beklenmeyen token alanları: %+v", resp)
	}
	if resp.User.Role != "hat_sefi" || resp.User.ID != 7 {
		t.Errorf("beklenmeyen kullanıcı bilgisi: %+v", resp.User)
	}
	if strings.Contains(rec.Body.String(), "gizli-ozet") || strings.Contains(strings.ToLower(rec.Body.String()), "password") {
		t.Error("yanıtta parola ya da parola özeti bulunmamalı")
	}
}

func TestLoginHandler_InvalidCredentials(t *testing.T) {
	handler := LoginHandler(fakeAuthenticator{err: auth.ErrInvalidCredentials})

	rec := postLogin(handler, `{"email":"a@b.test","password":"yanlis"}`)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("401 beklenirdi, gelen %d", rec.Code)
	}
	if code := decodeError(t, rec).Error.Code; code != "invalid_credentials" {
		t.Errorf("invalid_credentials beklenirdi, gelen %q", code)
	}
}

func TestLoginHandler_MalformedJSON(t *testing.T) {
	rec := postLogin(LoginHandler(fakeAuthenticator{}), `{bozuk json`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("400 beklenirdi, gelen %d", rec.Code)
	}
	if code := decodeError(t, rec).Error.Code; code != "invalid_request" {
		t.Errorf("invalid_request beklenirdi, gelen %q", code)
	}
}

func TestLoginHandler_MissingFields(t *testing.T) {
	bodies := map[string]string{
		"boş nesne":   `{}`,
		"parola yok":  `{"email":"a@b.test"}`,
		"e-posta yok": `{"password":"parola"}`,
		"boşluk":      `{"email":"   ","password":"parola"}`,
	}
	for name, body := range bodies {
		t.Run(name, func(t *testing.T) {
			rec := postLogin(LoginHandler(fakeAuthenticator{}), body)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("400 beklenirdi, gelen %d", rec.Code)
			}
			if code := decodeError(t, rec).Error.Code; code != "validation_error" {
				t.Errorf("validation_error beklenirdi, gelen %q", code)
			}
		})
	}
}

func TestLoginHandler_RejectsUnknownFields(t *testing.T) {
	rec := postLogin(LoginHandler(fakeAuthenticator{}), `{"email":"a@b.test","password":"p","role":"fabrika_muduru"}`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bilinmeyen alan için 400 beklenirdi, gelen %d", rec.Code)
	}
}

func TestLoginHandler_InternalErrorDoesNotLeakDetails(t *testing.T) {
	handler := LoginHandler(fakeAuthenticator{err: errors.New("veritabanı bağlantısı koptu: 10.0.0.5")})

	rec := postLogin(handler, `{"email":"a@b.test","password":"parola"}`)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("500 beklenirdi, gelen %d", rec.Code)
	}
	envelope := decodeError(t, rec)
	if envelope.Error.Code != "internal_error" {
		t.Errorf("internal_error beklenirdi, gelen %q", envelope.Error.Code)
	}
	if strings.Contains(rec.Body.String(), "10.0.0.5") || strings.Contains(rec.Body.String(), "veritabanı") {
		t.Error("iç hata ayrıntısı kullanıcıya sızmamalı")
	}
}
