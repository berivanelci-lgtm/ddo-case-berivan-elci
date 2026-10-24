// Package config, uygulama ayarlarını ortam değişkenlerinden okur.
package config

import (
	"fmt"
	"os"
)

// Config, uygulamanın çalışması için gereken ayarları tutar.
type Config struct {
	HTTPAddr    string
	DatabaseURL string
}

// Load, ayarları ortam değişkenlerinden okur.
// Zorunlu bir ayar eksikse hata döner.
func Load() (Config, error) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL ortam değişkeni tanımlı değil")
	}

	return Config{
		HTTPAddr:    getEnv("HTTP_ADDR", ":8080"),
		DatabaseURL: databaseURL,
	}, nil
}

// getEnv, ortam değişkenini okur; boşsa varsayılan değeri döner.
func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
