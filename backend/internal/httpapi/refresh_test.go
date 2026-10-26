package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/auth"
	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/user"
)

type fakeRefresher struct {
	err      error
	gotToken string
}

func (f *fakeRefresher) Refresh(_ context.Context, token string) (user.User, auth.TokenPair, error) {
	f.gotToken = token
	if f.err != nil {
		return user.User{}, auth.TokenPair{}, f.err
	}
	return user.User{ID: 3, Email: "kalite@hatkontrol.test", Role: user.RoleKaliteMuhendisi},
		auth.TokenPair{AccessToken: "yeni-access", RefreshToken: "yeni-refresh"}, nil
}

func postRefresh(f *fakeRefresher, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	RefreshHandler(f)(rec, httptest.NewRequest(http.MethodPost, "/auth/refresh", strings.NewReader(body)))
	return rec
}

func TestRefreshHandler_Success(t *testing.T) {
	f := &fakeRefresher{}
	rec := postRefresh(f, `{"refresh_token":"eski-refresh"}`)

	if rec.Code != http.StatusOK || f.gotToken != "eski-refresh" {
		t.Fatalf("200 ve token iletimi beklenirdi: %d %q", rec.Code, f.gotToken)
	}
	var resp loginResponse
	decodeInto(t, rec, &resp)
	if resp.AccessToken != "yeni-access" || resp.RefreshToken != "yeni-refresh" || resp.User.Role != "kalite_muhendisi" {
		t.Errorf("beklenmeyen yanıt: %+v", resp)
	}
}

func TestRefreshHandler_Errors(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		body   string
		status int
	}{
		{"boş token", nil, `{"refresh_token":""}`, 400},
		{"bozuk gövde", nil, `{bozuk`, 400},
		{"geçersiz token", auth.ErrInvalidToken, `{"refresh_token":"x"}`, 401},
		{"beklenmeyen", errors.New("bağlantı koptu: 10.0.0.17"), `{"refresh_token":"x"}`, 500},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := postRefresh(&fakeRefresher{err: tc.err}, tc.body)
			if rec.Code != tc.status {
				t.Fatalf("%d beklenirdi, gelen %d", tc.status, rec.Code)
			}
			if strings.Contains(rec.Body.String(), "10.0.0.17") {
				t.Error("iç hata sızmamalı")
			}
		})
	}
}
