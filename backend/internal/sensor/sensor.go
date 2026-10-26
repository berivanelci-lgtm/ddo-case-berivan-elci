// Package sensor, yüksek hacimli sensör okumalarının doğrulamasını ve yazılmasını yönetir.
package sensor

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"time"
)

const (
	// MaxBatch, tek istekte kabul edilen en fazla okuma sayısıdır.
	MaxBatch     = 1000
	maxStationID = 1000
	maxAge       = 90 * 24 * time.Hour
	clockSkew    = 5 * time.Minute
)

// ErrInvalid, okumalar geçersiz olduğunda döner.
var ErrInvalid = errors.New("sensör verisi geçersiz")

// Reading, bir istasyonun belirli andaki ölçüm değeridir.
type Reading struct {
	StationID  int32
	RecordedAt time.Time
	Value      float64
}

// Validate, toplu okumaları doğrular.
func Validate(readings []Reading, now time.Time) error {
	if len(readings) == 0 || len(readings) > MaxBatch {
		return fmt.Errorf("%w: bir istekte 1 ile %d okuma olmalı", ErrInvalid, MaxBatch)
	}
	for i, r := range readings {
		switch {
		case r.StationID < 1 || r.StationID > maxStationID:
			return fmt.Errorf("%w: %d. okumada istasyon 1 ile %d arasında olmalı", ErrInvalid, i+1, maxStationID)
		case r.RecordedAt.IsZero():
			return fmt.Errorf("%w: %d. okumada recorded_at zorunlu", ErrInvalid, i+1)
		case r.RecordedAt.After(now.Add(clockSkew)):
			return fmt.Errorf("%w: %d. okuma gelecekte", ErrInvalid, i+1)
		case r.RecordedAt.Before(now.Add(-maxAge)):
			return fmt.Errorf("%w: %d. okuma 90 günden eski", ErrInvalid, i+1)
		case math.IsNaN(r.Value) || math.IsInf(r.Value, 0):
			return fmt.Errorf("%w: %d. okumanın değeri sonlu bir sayı olmalı", ErrInvalid, i+1)
		}
	}
	return nil
}

// Months, okumaların düştüğü ayların başlangıçlarını (UTC) sıralı ve tekil olarak döner.
func Months(readings []Reading) []time.Time {
	seen := make(map[time.Time]bool)
	var months []time.Time
	for _, r := range readings {
		t := r.RecordedAt.UTC()
		month := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
		if !seen[month] {
			seen[month] = true
			months = append(months, month)
		}
	}
	sort.Slice(months, func(i, j int) bool { return months[i].Before(months[j]) })
	return months
}

// Store, okumaları toplu yazan depodur.
type Store interface {
	Insert(ctx context.Context, readings []Reading) (int64, error)
}

// Service, okumaları doğrulayıp yazar.
type Service struct {
	store Store
	now   func() time.Time
}

// NewService, yeni bir Service oluşturur.
func NewService(store Store) *Service {
	return &Service{store: store, now: time.Now}
}

// Ingest, okumaları doğrular ve tek seferde yazar; yazılan satır sayısını döner.
func (s *Service) Ingest(ctx context.Context, readings []Reading) (int64, error) {
	if err := Validate(readings, s.now()); err != nil {
		return 0, err
	}
	return s.store.Insert(ctx, readings)
}
