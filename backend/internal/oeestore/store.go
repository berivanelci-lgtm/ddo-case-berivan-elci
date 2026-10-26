// Package oeestore, hat, duruş ve sayaç kayıtlarını PostgreSQL'de saklar.
package oeestore

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/audit"
	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/oee"
)

const (
	uniqueViolation     = "23505"
	foreignKeyViolation = "23503"
)

// Store, oee.Repository arayüzünü PostgreSQL ile gerçekler.
type Store struct {
	pool *pgxpool.Pool
}

var _ oee.Repository = (*Store)(nil)

// New, yeni bir Store oluşturur.
func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func hasCode(err error, code string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == code
}

// CreateLine, yeni bir hat ekler.
func (s *Store) CreateLine(ctx context.Context, code, name string, idealCycle time.Duration) (oee.Line, error) {
	tx, err := audit.Begin(ctx, s.pool)
	if err != nil {
		return oee.Line{}, fmt.Errorf("işlem başlatılamadı: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	line := oee.Line{Code: code, Name: name, IdealCycle: idealCycle}
	err = tx.QueryRow(ctx, `INSERT INTO lines (code, name, ideal_cycle_ms) VALUES ($1, $2, $3) RETURNING id`,
		code, name, idealCycle.Milliseconds()).Scan(&line.ID)
	if hasCode(err, uniqueViolation) {
		return oee.Line{}, oee.ErrLineExists
	}
	if err != nil {
		return oee.Line{}, fmt.Errorf("hat oluşturulamadı: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return oee.Line{}, fmt.Errorf("işlem onaylanamadı: %w", err)
	}
	return line, nil
}

// ListLines, hatları kimlik sırasıyla döner.
func (s *Store) ListLines(ctx context.Context) ([]oee.Line, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, code, name, ideal_cycle_ms FROM lines ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("hatlar sorgulanamadı: %w", err)
	}
	defer rows.Close()

	lines := make([]oee.Line, 0)
	for rows.Next() {
		var (
			line oee.Line
			ms   int64
		)
		if err := rows.Scan(&line.ID, &line.Code, &line.Name, &ms); err != nil {
			return nil, fmt.Errorf("hat okunamadı: %w", err)
		}
		line.IdealCycle = time.Duration(ms) * time.Millisecond
		lines = append(lines, line)
	}
	return lines, rows.Err()
}

// FindLine, hattı koduyla bulur.
func (s *Store) FindLine(ctx context.Context, code string) (oee.Line, error) {
	var (
		line oee.Line
		ms   int64
	)
	err := s.pool.QueryRow(ctx, `SELECT id, code, name, ideal_cycle_ms FROM lines WHERE code = $1`, code).
		Scan(&line.ID, &line.Code, &line.Name, &ms)
	if errors.Is(err, pgx.ErrNoRows) {
		return oee.Line{}, oee.ErrLineNotFound
	}
	if err != nil {
		return oee.Line{}, fmt.Errorf("hat sorgulanamadı: %w", err)
	}
	line.IdealCycle = time.Duration(ms) * time.Millisecond
	return line, nil
}

// Reasons, duruş nedenlerini döner.
func (s *Store) Reasons(ctx context.Context) ([]oee.Reason, error) {
	rows, err := s.pool.Query(ctx, `SELECT code, description, planned FROM downtime_reasons ORDER BY code`)
	if err != nil {
		return nil, fmt.Errorf("duruş nedenleri sorgulanamadı: %w", err)
	}
	defer rows.Close()

	reasons := make([]oee.Reason, 0)
	for rows.Next() {
		var reason oee.Reason
		if err := rows.Scan(&reason.Code, &reason.Description, &reason.Planned); err != nil {
			return nil, fmt.Errorf("duruş nedeni okunamadı: %w", err)
		}
		reasons = append(reasons, reason)
	}
	return reasons, rows.Err()
}

// AddDowntime, bir duruş olayı ekler. Tanımsız neden ErrUnknownReason döner.
func (s *Store) AddDowntime(ctx context.Context, lineID int64, reasonCode string, start, end time.Time, userID int64) (int64, error) {
	tx, err := audit.Begin(ctx, s.pool)
	if err != nil {
		return 0, fmt.Errorf("işlem başlatılamadı: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var id int64
	err = tx.QueryRow(ctx, `
		INSERT INTO downtime_events (line_id, reason_code, started_at, ended_at, recorded_by)
		VALUES ($1, $2, $3, $4, $5) RETURNING id`, lineID, reasonCode, start, end, userID).Scan(&id)
	if hasCode(err, foreignKeyViolation) {
		return 0, oee.ErrUnknownReason
	}
	if err != nil {
		return 0, fmt.Errorf("duruş kaydedilemedi: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("işlem onaylanamadı: %w", err)
	}
	return id, nil
}

// AddCount, bir üretim sayacı ekler.
func (s *Store) AddCount(ctx context.Context, lineID int64, at time.Time, good, scrap, userID int64) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO production_counts (line_id, occurred_at, good, scrap, recorded_by)
		VALUES ($1, $2, $3, $4, $5) RETURNING id`, lineID, at, good, scrap, userID).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("sayaç kaydedilemedi: %w", err)
	}
	return id, nil
}

// Events, [from, to) ile kesişen duruşları ve penceredeki sayaçları döner.
// Bir duruş en fazla 24 saat sürdüğü için başlangıç dizini from-24 saatten itibaren taranır.
func (s *Store) Events(ctx context.Context, lineID int64, from, to time.Time) ([]oee.Downtime, []oee.Count, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT d.started_at, d.ended_at, r.planned
		FROM downtime_events d
		JOIN downtime_reasons r ON r.code = d.reason_code
		WHERE d.line_id = $1
		  AND d.started_at >= $2::timestamptz - interval '24 hours'
		  AND d.started_at < $3
		  AND d.ended_at > $2
		ORDER BY d.started_at, d.id`, lineID, from, to)
	if err != nil {
		return nil, nil, fmt.Errorf("duruşlar sorgulanamadı: %w", err)
	}
	var downtimes []oee.Downtime
	for rows.Next() {
		var d oee.Downtime
		if err := rows.Scan(&d.Start, &d.End, &d.Planned); err != nil {
			rows.Close()
			return nil, nil, fmt.Errorf("duruş okunamadı: %w", err)
		}
		downtimes = append(downtimes, d)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("duruşlar okunamadı: %w", err)
	}

	countRows, err := s.pool.Query(ctx, `
		SELECT occurred_at, good, scrap
		FROM production_counts
		WHERE line_id = $1 AND occurred_at >= $2 AND occurred_at < $3
		ORDER BY occurred_at, id`, lineID, from, to)
	if err != nil {
		return nil, nil, fmt.Errorf("sayaçlar sorgulanamadı: %w", err)
	}
	defer countRows.Close()
	var counts []oee.Count
	for countRows.Next() {
		var c oee.Count
		if err := countRows.Scan(&c.At, &c.Good, &c.Scrap); err != nil {
			return nil, nil, fmt.Errorf("sayaç okunamadı: %w", err)
		}
		counts = append(counts, c)
	}
	return downtimes, counts, countRows.Err()
}
