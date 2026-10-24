package auth

import (
	"errors"
	"strings"
	"testing"
	"time"
)

const testSecret = "test-icin-uzun-bir-gizli-anahtar-0123456789"

func newTestManager(t *testing.T) *TokenManager {
	t.Helper()
	m, err := NewTokenManager(testSecret, 15*time.Minute, 24*time.Hour)
	if err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}
	return m
}

func TestNewTokenManager_RejectsShortSecret(t *testing.T) {
	if _, err := NewTokenManager("kisa", time.Minute, time.Hour); err == nil {
		t.Fatal("kısa gizli anahtar reddedilmeli")
	}
}

func TestIssueAndParse_RoundTrip(t *testing.T) {
	m := newTestManager(t)

	token, err := m.Issue(42, "hat_sefi", AccessToken)
	if err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}

	claims, err := m.Parse(token, AccessToken)
	if err != nil {
		t.Fatalf("geçerli token kabul edilmeli: %v", err)
	}
	if claims.UserID != 42 || claims.Role != "hat_sefi" {
		t.Fatalf("beklenmeyen içerik: %+v", claims)
	}
}

func TestParse_RejectsExpiredToken(t *testing.T) {
	m := newTestManager(t)
	start := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)

	m.now = func() time.Time { return start }
	token, err := m.Issue(1, "operator", AccessToken)
	if err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}

	m.now = func() time.Time { return start.Add(16 * time.Minute) }
	if _, err := m.Parse(token, AccessToken); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("süresi dolmuş token reddedilmeli, gelen: %v", err)
	}
}

func TestParse_RejectsWrongSecret(t *testing.T) {
	issuer := newTestManager(t)
	other, err := NewTokenManager("baska-bir-uzun-gizli-anahtar-9876543210", time.Minute, time.Hour)
	if err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}

	token, err := issuer.Issue(1, "operator", AccessToken)
	if err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}
	if _, err := other.Parse(token, AccessToken); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("başka anahtarla imzalı token reddedilmeli, gelen: %v", err)
	}
}

func TestParse_RejectsWrongKind(t *testing.T) {
	m := newTestManager(t)

	refresh, err := m.Issue(1, "operator", RefreshToken)
	if err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}
	if _, err := m.Parse(refresh, AccessToken); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("refresh token, access yerine kabul edilmemeli, gelen: %v", err)
	}
}

func TestParse_RejectsForgedPayload(t *testing.T) {
	m := newTestManager(t)

	operatorToken, err := m.Issue(1, "operator", AccessToken)
	if err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}
	adminToken, err := m.Issue(2, "fabrika_muduru", AccessToken)
	if err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}

	operatorParts := strings.Split(operatorToken, ".")
	adminParts := strings.Split(adminToken, ".")
	forged := adminParts[0] + "." + adminParts[1] + "." + operatorParts[2]

	if _, err := m.Parse(forged, AccessToken); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("içeriği değiştirilmiş token reddedilmeli, gelen: %v", err)
	}
}
