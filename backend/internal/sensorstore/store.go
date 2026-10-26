// Package sensorstore, sensör okumalarını aylık partisyonlu tabloya COPY ile yazar.
package sensorstore

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/sensor"
)

// Store, sensor.Store arayüzünü PostgreSQL ile gerçekler.
type Store struct {
	pool  *pgxpool.Pool
	mu    sync.Mutex
	ready map[time.Time]bool // bu süreçte varlığı doğrulanmış ay partisyonları
}

var _ sensor.Store = (*Store)(nil)

// New, yeni bir Store oluşturur.
func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool, ready: make(map[time.Time]bool)}
}

// Insert, okumaların aylarına ait partisyonları hazırlar ve hepsini tek bir COPY ile yazar.
// COPY tek komut olduğu için toplu yazma ya tamamen olur ya da hiç olmaz.
func (s *Store) Insert(ctx context.Context, readings []sensor.Reading) (int64, error) {
	if err := s.ensurePartitions(ctx, sensor.Months(readings)); err != nil {
		return 0, err
	}

	rows := make([][]any, 0, len(readings))
	for _, r := range readings {
		rows = append(rows, []any{r.StationID, r.RecordedAt.UTC(), r.Value})
	}
	n, err := s.pool.CopyFrom(ctx, pgx.Identifier{"sensor_readings"},
		[]string{"station_id", "recorded_at", "value"}, pgx.CopyFromRows(rows))
	if err != nil {
		return 0, fmt.Errorf("okumalar yazılamadı: %w", err)
	}
	return n, nil
}

// ensurePartitions, her ay için partisyonu bir kez (süreç ömrü boyunca) hazırlar.
// Partisyon oluşturma kendi kısa işleminde yapılır, böylece COPY işlemi kilit beklemez.
func (s *Store) ensurePartitions(ctx context.Context, months []time.Time) error {
	for _, month := range months {
		s.mu.Lock()
		known := s.ready[month]
		s.mu.Unlock()
		if known {
			continue
		}
		if _, err := s.pool.Exec(ctx, `SELECT ensure_sensor_partition($1)`, month); err != nil {
			return fmt.Errorf("partisyon hazırlanamadı: %w", err)
		}
		s.mu.Lock()
		s.ready[month] = true
		s.mu.Unlock()
	}
	return nil
}
