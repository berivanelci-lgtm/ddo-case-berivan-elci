// Package config, uygulama ayarlarını ortam değişkenlerinden okur.
package config

import (
	"fmt"
	"os"
	"time"
)

const (
	defaultHTTPAddr   = ":8080"
	defaultAccessTTL  = 15 * time.Minute
	defaultRefreshTTL = 24 * time.Hour
)

// Config, uygulamanın çalışması için gereken ayarları tutar.
type Config struct {
	HTTPAddr     string
	DatabaseURL  string
	JWTSecret    string
	AccessTTL    time.Duration
	RefreshTTL   time.Duration
	DemoPassword string
}

// Load, ayarları ortam değişkenlerinden okur.
// Zorunlu bir ayar eksikse ya da geçersizse hata döner.
func Load() (Config, error) {
	databaseURL, err := required("DATABASE_URL")
	if err != nil {
		return Config{}, err
	}
	jwtSecret, err := required("JWT_SECRET")
	if err != nil {
		return Config{}, err
	}
	demoPassword, err := required("DEMO_PASSWORD")
	if err != nil {
		return Config{}, err
	}
	accessTTL, err := durationEnv("ACCESS_TOKEN_TTL", defaultAccessTTL)
	if err != nil {
		return Config{}, err
	}
	refreshTTL, err := durationEnv("REFRESH_TOKEN_TTL", defaultRefreshTTL)
	if err != nil {
		return Config{}, err
	}

	return Config{
		HTTPAddr:     getEnv("HTTP_ADDR", defaultHTTPAddr),
		DatabaseURL:  databaseURL,
		JWTSecret:    jwtSecret,
		AccessTTL:    accessTTL,
		RefreshTTL:   refreshTTL,
		DemoPassword: demoPassword,
	}, nil
}

// required, zorunlu bir ortam değişkenini okur; boşsa hata döner.
func required(key string) (string, error) {
	value := os.Getenv(key)
	if value == "" {
		return "", fmt.Errorf("%s ortam değişkeni tanımlı değil", key)
	}
	return value, nil
}

// getEnv, ortam değişkenini okur; boşsa varsayılan değeri döner.
func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

// durationEnv, süre değeri taşıyan ortam değişkenini okur (örn. "15m").
func durationEnv(key string, fallback time.Duration) (time.Duration, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback, nil
	}
	value, err := time.ParseDuration(raw)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("%s geçerli bir süre olmalı (örn. 15m): %q", key, raw)
	}
	return value, nil
}
