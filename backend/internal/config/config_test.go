package config

import (
	"testing"
	"time"
)

// setRequiredEnv, zorunlu ayarların hepsini geçerli değerlerle tanımlar.
func setRequiredEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://u:p@db:5432/x")
	t.Setenv("JWT_SECRET", "test-icin-uzun-bir-gizli-anahtar-0123456789")
	t.Setenv("DEMO_PASSWORD", "demo-parola")
}

func TestLoad_UsesDefaults(t *testing.T) {
	setRequiredEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}
	if cfg.HTTPAddr != ":8080" {
		t.Errorf("varsayılan adres :8080 olmalı, gelen %q", cfg.HTTPAddr)
	}
	if cfg.AccessTTL != 15*time.Minute {
		t.Errorf("varsayılan access süresi 15dk olmalı, gelen %v", cfg.AccessTTL)
	}
	if cfg.RefreshTTL != 24*time.Hour {
		t.Errorf("varsayılan refresh süresi 24sa olmalı, gelen %v", cfg.RefreshTTL)
	}
}

func TestLoad_ReadsCustomValues(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("HTTP_ADDR", ":9090")
	t.Setenv("ACCESS_TOKEN_TTL", "5m")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}
	if cfg.HTTPAddr != ":9090" {
		t.Errorf("adres :9090 olmalı, gelen %q", cfg.HTTPAddr)
	}
	if cfg.AccessTTL != 5*time.Minute {
		t.Errorf("access süresi 5dk olmalı, gelen %v", cfg.AccessTTL)
	}
}

func TestLoad_FailsWhenRequiredSettingMissing(t *testing.T) {
	for _, missing := range []string{"DATABASE_URL", "JWT_SECRET", "DEMO_PASSWORD"} {
		t.Run(missing, func(t *testing.T) {
			setRequiredEnv(t)
			t.Setenv(missing, "")

			if _, err := Load(); err == nil {
				t.Fatalf("%s eksikken hata dönmeli", missing)
			}
		})
	}
}

func TestLoad_FailsOnInvalidDuration(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("ACCESS_TOKEN_TTL", "yarim-saat")

	if _, err := Load(); err == nil {
		t.Fatal("geçersiz süre için hata dönmeli")
	}
}

func TestLoad_FailsOnNonPositiveDuration(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("REFRESH_TOKEN_TTL", "-1h")

	if _, err := Load(); err == nil {
		t.Fatal("negatif süre için hata dönmeli")
	}
}
