// Package qualitystore, kalite formlarını ve muayeneleri PostgreSQL'de saklar.
package qualitystore

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/audit"
	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/quality"
)

const (
	uniqueViolation     = "23505"
	foreignKeyViolation = "23503"
)

// Store, quality.Repository arayüzünü PostgreSQL ile gerçekler.
type Store struct {
	pool *pgxpool.Pool
}

var _ quality.Repository = (*Store)(nil)

// New, yeni bir Store oluşturur.
func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func hasCode(err error, code string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == code
}

// CreateForm, formu ve karakteristiklerini tek işlemde ekler.
func (s *Store) CreateForm(ctx context.Context, f quality.Form, createdBy int64) (quality.Form, error) {
	tx, err := audit.Begin(ctx, s.pool)
	if err != nil {
		return quality.Form{}, fmt.Errorf("işlem başlatılamadı: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	err = tx.QueryRow(ctx, `
		INSERT INTO quality_forms (code, name, gauge_code, calibration_due, created_by)
		VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		f.Code, f.Name, f.GaugeCode, f.CalibrationDue, createdBy).Scan(&f.ID)
	if hasCode(err, uniqueViolation) {
		return quality.Form{}, quality.ErrExists
	}
	if err != nil {
		return quality.Form{}, fmt.Errorf("form oluşturulamadı: %w", err)
	}

	for _, item := range f.Items {
		_, err := tx.Exec(ctx, `
			INSERT INTO quality_form_items (form_id, position, characteristic, unit, nominal, lsl, usl, chart_id)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			f.ID, item.Position, item.Characteristic, item.Unit, item.Nominal, item.LSL, item.USL, item.ChartID)
		if hasCode(err, foreignKeyViolation) {
			return quality.Form{}, fmt.Errorf("%w: bağlı SPC grafiği bulunamadı", quality.ErrInvalid)
		}
		if err != nil {
			return quality.Form{}, fmt.Errorf("karakteristik eklenemedi: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return quality.Form{}, fmt.Errorf("işlem onaylanamadı: %w", err)
	}
	return f, nil
}

// ListForms, formları karakteristikleriyle döner.
func (s *Store) ListForms(ctx context.Context) ([]quality.Form, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, code, name, gauge_code, calibration_due FROM quality_forms ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("formlar sorgulanamadı: %w", err)
	}
	forms := make([]quality.Form, 0)
	index := make(map[int64]int)
	for rows.Next() {
		var f quality.Form
		if err := rows.Scan(&f.ID, &f.Code, &f.Name, &f.GaugeCode, &f.CalibrationDue); err != nil {
			rows.Close()
			return nil, fmt.Errorf("form okunamadı: %w", err)
		}
		index[f.ID] = len(forms)
		forms = append(forms, f)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("formlar okunamadı: %w", err)
	}

	items, err := s.items(ctx, `SELECT form_id, position, characteristic, unit, nominal, lsl, usl, chart_id
		FROM quality_form_items ORDER BY form_id, position`)
	if err != nil {
		return nil, err
	}
	for formID, list := range items {
		if i, ok := index[formID]; ok {
			forms[i].Items = list
		}
	}
	return forms, nil
}

// GetForm, formu karakteristikleriyle döner.
func (s *Store) GetForm(ctx context.Context, id int64) (quality.Form, error) {
	var f quality.Form
	err := s.pool.QueryRow(ctx, `SELECT id, code, name, gauge_code, calibration_due FROM quality_forms WHERE id = $1`, id).
		Scan(&f.ID, &f.Code, &f.Name, &f.GaugeCode, &f.CalibrationDue)
	if errors.Is(err, pgx.ErrNoRows) {
		return quality.Form{}, quality.ErrNotFound
	}
	if err != nil {
		return quality.Form{}, fmt.Errorf("form sorgulanamadı: %w", err)
	}

	items, err := s.items(ctx, `SELECT form_id, position, characteristic, unit, nominal, lsl, usl, chart_id
		FROM quality_form_items WHERE form_id = $1 ORDER BY position`, id)
	if err != nil {
		return quality.Form{}, err
	}
	f.Items = items[id]
	return f, nil
}

func (s *Store) items(ctx context.Context, query string, args ...any) (map[int64][]quality.Item, error) {
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("karakteristikler sorgulanamadı: %w", err)
	}
	defer rows.Close()

	result := make(map[int64][]quality.Item)
	for rows.Next() {
		var (
			formID int64
			item   quality.Item
		)
		if err := rows.Scan(&formID, &item.Position, &item.Characteristic, &item.Unit,
			&item.Nominal, &item.LSL, &item.USL, &item.ChartID); err != nil {
			return nil, fmt.Errorf("karakteristik okunamadı: %w", err)
		}
		result[formID] = append(result[formID], item)
	}
	return result, rows.Err()
}

// SaveInspection, muayeneyi ve değerlerini tek işlemde kaydeder.
func (s *Store) SaveInspection(ctx context.Context, in quality.Inspection) (quality.Inspection, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return quality.Inspection{}, fmt.Errorf("işlem başlatılamadı: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	err = tx.QueryRow(ctx, `
		INSERT INTO quality_inspections (form_id, inspected_by, work_order_id, all_in_spec)
		VALUES ($1, $2, $3, $4) RETURNING id, inspected_at`,
		in.FormID, in.InspectedBy, in.WorkOrderID, in.AllInSpec).Scan(&in.ID, &in.InspectedAt)
	if hasCode(err, foreignKeyViolation) {
		return quality.Inspection{}, fmt.Errorf("%w: iş emri bulunamadı", quality.ErrInvalid)
	}
	if err != nil {
		return quality.Inspection{}, fmt.Errorf("muayene kaydedilemedi: %w", err)
	}

	for _, v := range in.Values {
		if _, err := tx.Exec(ctx, `
			INSERT INTO quality_inspection_values (inspection_id, position, value, in_spec)
			VALUES ($1, $2, $3, $4)`, in.ID, v.Position, v.Value, v.InSpec); err != nil {
			return quality.Inspection{}, fmt.Errorf("ölçüm değeri kaydedilemedi: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return quality.Inspection{}, fmt.Errorf("işlem onaylanamadı: %w", err)
	}
	return in, nil
}

// RecentInspections, formun son muayenelerini yeniden eskiye döner.
func (s *Store) RecentInspections(ctx context.Context, formID int64, limit int) ([]quality.Inspection, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, form_id, inspected_by, inspected_at, work_order_id, all_in_spec
		FROM quality_inspections WHERE form_id = $1 ORDER BY id DESC LIMIT $2`, formID, limit)
	if err != nil {
		return nil, fmt.Errorf("muayeneler sorgulanamadı: %w", err)
	}
	inspections := make([]quality.Inspection, 0)
	index := make(map[int64]int)
	ids := make([]int64, 0)
	for rows.Next() {
		var in quality.Inspection
		var inspectedAt time.Time
		if err := rows.Scan(&in.ID, &in.FormID, &in.InspectedBy, &inspectedAt, &in.WorkOrderID, &in.AllInSpec); err != nil {
			rows.Close()
			return nil, fmt.Errorf("muayene okunamadı: %w", err)
		}
		in.InspectedAt = inspectedAt
		index[in.ID] = len(inspections)
		ids = append(ids, in.ID)
		inspections = append(inspections, in)
	}
	rows.Close()
	if err := rows.Err(); err != nil || len(ids) == 0 {
		return inspections, err
	}

	valueRows, err := s.pool.Query(ctx, `
		SELECT v.inspection_id, v.position, i.characteristic, v.value, v.in_spec
		FROM quality_inspection_values v
		JOIN quality_form_items i ON i.form_id = $2 AND i.position = v.position
		WHERE v.inspection_id = ANY($1)
		ORDER BY v.inspection_id, v.position`, ids, formID)
	if err != nil {
		return nil, fmt.Errorf("ölçüm değerleri sorgulanamadı: %w", err)
	}
	defer valueRows.Close()
	for valueRows.Next() {
		var (
			inspectionID int64
			v            quality.Value
		)
		if err := valueRows.Scan(&inspectionID, &v.Position, &v.Characteristic, &v.Value, &v.InSpec); err != nil {
			return nil, fmt.Errorf("ölçüm değeri okunamadı: %w", err)
		}
		i := index[inspectionID]
		inspections[i].Values = append(inspections[i].Values, v)
	}
	return inspections, valueRows.Err()
}
