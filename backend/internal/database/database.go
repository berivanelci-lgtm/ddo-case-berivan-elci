// Package database, PostgreSQL bağlantısını ve migrasyonları yönetir.
package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib" // goose için "pgx" sürücüsünü kaydeder
	"github.com/pressly/goose/v3"

	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/migrations"
)

const connectTimeout = 10 * time.Second

// Connect, bir bağlantı havuzu açar ve veritabanına ulaşılabildiğini doğrular.
func Connect(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	ctx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("bağlantı havuzu açılamadı: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("veritabanına ulaşılamadı: %w", err)
	}

	return pool, nil
}

// Migrate, bekleyen tüm migrasyonları sırayla uygular.
func Migrate(databaseURL string) error {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return fmt.Errorf("migrasyon bağlantısı açılamadı: %w", err)
	}
	defer db.Close()

	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("goose dialect ayarlanamadı: %w", err)
	}

	if err := goose.Up(db, "."); err != nil {
		return fmt.Errorf("migrasyonlar uygulanamadı: %w", err)
	}

	return nil
}
