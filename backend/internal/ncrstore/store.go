// Package ncrstore, uygunsuzluk kayıtlarını PostgreSQL'de saklar.
package ncrstore

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/audit"
	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/ncr"
)

const (
	foreignKeyViolation = "23503"
	columns             = `id, title, description, severity, status, work_order_id, chart_id, opened_by,
		root_cause, corrective_action, closed_by, created_at, closed_at`
)

// Store, ncr.Repository arayüzünü PostgreSQL ile gerçekler.
type Store struct {
	pool *pgxpool.Pool
}

var _ ncr.Repository = (*Store)(nil)

// New, yeni bir Store oluşturur.
func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// Create, kaydı ekler. Bağlı iş emri ya da grafik yoksa ErrInvalid döner.
func (s *Store) Create(ctx context.Context, n ncr.NCR) (ncr.NCR, error) {
	tx, err := audit.Begin(ctx, s.pool)
	if err != nil {
		return ncr.NCR{}, fmt.Errorf("işlem başlatılamadı: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const query = `
		INSERT INTO ncrs (title, description, severity, status, work_order_id, chart_id, opened_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at`
	err = tx.QueryRow(ctx, query, n.Title, n.Description, string(n.Severity), string(n.Status),
		n.WorkOrderID, n.ChartID, n.OpenedBy).Scan(&n.ID, &n.CreatedAt)

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == foreignKeyViolation {
		return ncr.NCR{}, fmt.Errorf("%w: bağlı iş emri ya da grafik bulunamadı", ncr.ErrInvalid)
	}
	if err != nil {
		return ncr.NCR{}, fmt.Errorf("uygunsuzluk kaydı oluşturulamadı: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return ncr.NCR{}, fmt.Errorf("işlem onaylanamadı: %w", err)
	}
	return n, nil
}

// Get, kaydı kimliğiyle getirir.
func (s *Store) Get(ctx context.Context, id int64) (ncr.NCR, error) {
	return scanNCR(s.pool.QueryRow(ctx, `SELECT `+columns+` FROM ncrs WHERE id = $1`, id))
}

// List, kayıtları yeniden eskiye sıralı getirir (keyset sayfalama).
func (s *Store) List(ctx context.Context, status ncr.Status, before int64, limit int) ([]ncr.NCR, error) {
	const query = `
		SELECT ` + columns + `
		FROM ncrs
		WHERE ($1::text = '' OR status = $1::text)
		  AND ($2::bigint = 0 OR id < $2::bigint)
		ORDER BY id DESC
		LIMIT $3`

	rows, err := s.pool.Query(ctx, query, string(status), before, limit)
	if err != nil {
		return nil, fmt.Errorf("uygunsuzluk kayıtları sorgulanamadı: %w", err)
	}
	defer rows.Close()

	var items []ncr.NCR
	for rows.Next() {
		n, err := scanNCR(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("uygunsuzluk kayıtları okunamadı: %w", err)
	}
	return items, nil
}

// Update, kaydı kilitleyip change ile değiştirir ve tek işlemde kaydeder.
func (s *Store) Update(ctx context.Context, id int64, change func(ncr.NCR) (ncr.NCR, error)) (ncr.NCR, error) {
	tx, err := audit.Begin(ctx, s.pool)
	if err != nil {
		return ncr.NCR{}, fmt.Errorf("işlem başlatılamadı: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	current, err := scanNCR(tx.QueryRow(ctx, `SELECT `+columns+` FROM ncrs WHERE id = $1 FOR UPDATE`, id))
	if err != nil {
		return ncr.NCR{}, err
	}
	changed, err := change(current)
	if err != nil {
		return ncr.NCR{}, err
	}

	const query = `
		UPDATE ncrs
		SET status = $2, root_cause = $3, corrective_action = $4, closed_by = $5, closed_at = $6, updated_at = now()
		WHERE id = $1`
	_, err = tx.Exec(ctx, query, id, string(changed.Status), nullable(changed.RootCause),
		nullable(changed.CorrectiveAction), changed.ClosedBy, changed.ClosedAt)
	if err != nil {
		return ncr.NCR{}, fmt.Errorf("uygunsuzluk kaydı güncellenemedi: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return ncr.NCR{}, fmt.Errorf("işlem onaylanamadı: %w", err)
	}
	return changed, nil
}

func nullable(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

type scanner interface {
	Scan(dest ...any) error
}

func scanNCR(row scanner) (ncr.NCR, error) {
	var (
		n                 ncr.NCR
		severity, status  string
		rootCause, action *string
	)
	err := row.Scan(&n.ID, &n.Title, &n.Description, &severity, &status, &n.WorkOrderID, &n.ChartID,
		&n.OpenedBy, &rootCause, &action, &n.ClosedBy, &n.CreatedAt, &n.ClosedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ncr.NCR{}, ncr.ErrNotFound
	}
	if err != nil {
		return ncr.NCR{}, fmt.Errorf("uygunsuzluk kaydı okunamadı: %w", err)
	}
	n.Severity = ncr.Severity(severity)
	n.Status = ncr.Status(status)
	if rootCause != nil {
		n.RootCause = *rootCause
	}
	if action != nil {
		n.CorrectiveAction = *action
	}
	return n, nil
}
