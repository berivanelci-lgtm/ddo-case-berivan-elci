// Package workorderstore, iş emirlerini ve malzeme stoklarını PostgreSQL'de saklar.
package workorderstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/audit"
	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/idempotency"
	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/workorder"
)

const (
	foreignKeyViolation = "23503"
	orderColumns        = `id, product_id, planned_qty, status, started_qty, good_qty, scrap_qty, rework_pending`

	statusOK      = 200
	statusCreated = 201
)

// Store, workorder.Store ve workorder.Repository arayüzlerini PostgreSQL ile gerçekler.
type Store struct {
	pool *pgxpool.Pool
}

var (
	_ workorder.Store      = (*Store)(nil)
	_ workorder.Repository = (*Store)(nil)
)

// New, yeni bir Store oluşturur.
func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// orderBody, iş emri yanıtının JSON biçimidir. httpapi.orderResponse ile aynı alanları taşır;
// idempotency kaydındaki yanıt, normal yanıtla birebir aynı olmalıdır.
type orderBody struct {
	ID            int64  `json:"id"`
	ProductID     int64  `json:"product_id"`
	PlannedQty    int64  `json:"planned_qty"`
	Status        string `json:"status"`
	StartedQty    int64  `json:"started_qty"`
	GoodQty       int64  `json:"good_qty"`
	ScrapQty      int64  `json:"scrap_qty"`
	ReworkPending int64  `json:"rework_pending"`
}

func toOrderBody(o workorder.WorkOrder) orderBody {
	return orderBody{
		ID:            o.ID,
		ProductID:     o.ProductID,
		PlannedQty:    o.PlannedQty,
		Status:        string(o.Status),
		StartedQty:    o.StartedQty,
		GoodQty:       o.GoodQty,
		ScrapQty:      o.ScrapQty,
		ReworkPending: o.ReworkPending,
	}
}

// recordIdempotency, context'te anahtar varsa yanıtı aynı transaction içinde kaydeder.
func recordIdempotency(ctx context.Context, q idempotency.Querier, status int, order workorder.WorkOrder) error {
	pending, ok := idempotency.PendingFrom(ctx)
	if !ok {
		return nil
	}
	body, err := json.Marshal(toOrderBody(order))
	if err != nil {
		return fmt.Errorf("yanıt kodlanamadı: %w", err)
	}
	return idempotency.Save(ctx, q, pending.UserID, pending.Key, pending.RequestHash, status, body)
}

// WithOrderLock, iş emrinin satırını kilitleyip fn'i tek bir veritabanı işlemi içinde çalıştırır.
// fn hata dönerse işlem geri alınır: malzeme düşümü, kayıtlar, durum ve idempotency kaydı birlikte iptal olur.
func (s *Store) WithOrderLock(ctx context.Context, orderID int64, fn func(workorder.Tx) error) error {
	tx, err := audit.Begin(ctx, s.pool)
	if err != nil {
		return fmt.Errorf("işlem başlatılamadı: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const lockQuery = `SELECT ` + orderColumns + ` FROM work_orders WHERE id = $1 FOR UPDATE`
	order, err := scanOrder(tx.QueryRow(ctx, lockQuery, orderID))
	if err != nil {
		return err
	}

	ot := &orderTx{q: tx, order: order}
	if err := fn(ot); err != nil {
		return err
	}
	if err := recordIdempotency(ctx, tx, statusOK, ot.finalOrder()); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("işlem onaylanamadı: %w", err)
	}
	return nil
}

// Create, planlanmış yeni bir iş emri oluşturur. Idempotency anahtarı varsa aynı işlemde kaydeder.
func (s *Store) Create(ctx context.Context, productID, plannedQty, createdBy int64) (workorder.WorkOrder, error) {
	order, err := workorder.New(productID, plannedQty)
	if err != nil {
		return workorder.WorkOrder{}, err
	}

	tx, err := audit.Begin(ctx, s.pool)
	if err != nil {
		return workorder.WorkOrder{}, fmt.Errorf("işlem başlatılamadı: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const query = `
		INSERT INTO work_orders (product_id, planned_qty, status, created_by)
		VALUES ($1, $2, $3, $4)
		RETURNING id`
	err = tx.QueryRow(ctx, query, productID, plannedQty, string(order.Status), createdBy).Scan(&order.ID)

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == foreignKeyViolation {
		return workorder.WorkOrder{}, workorder.ErrProductNotFound
	}
	if err != nil {
		return workorder.WorkOrder{}, fmt.Errorf("iş emri oluşturulamadı: %w", err)
	}

	if err := recordIdempotency(ctx, tx, statusCreated, order); err != nil {
		return workorder.WorkOrder{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return workorder.WorkOrder{}, fmt.Errorf("işlem onaylanamadı: %w", err)
	}
	return order, nil
}

// Get, iş emrini kimliğiyle getirir.
func (s *Store) Get(ctx context.Context, id int64) (workorder.WorkOrder, error) {
	const query = `SELECT ` + orderColumns + ` FROM work_orders WHERE id = $1`
	return scanOrder(s.pool.QueryRow(ctx, query, id))
}

// List, kimliği before'dan küçük iş emirlerini yeniden eskiye sıralı getirir (keyset sayfalama).
func (s *Store) List(ctx context.Context, before int64, limit int) ([]workorder.WorkOrder, error) {
	const query = `
		SELECT ` + orderColumns + `
		FROM work_orders
		WHERE $1::bigint = 0 OR id < $1::bigint
		ORDER BY id DESC
		LIMIT $2`

	rows, err := s.pool.Query(ctx, query, before, limit)
	if err != nil {
		return nil, fmt.Errorf("iş emirleri sorgulanamadı: %w", err)
	}
	defer rows.Close()

	var orders []workorder.WorkOrder
	for rows.Next() {
		order, err := scanOrder(rows)
		if err != nil {
			return nil, err
		}
		orders = append(orders, order)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iş emirleri okunamadı: %w", err)
	}
	return orders, nil
}

// orderTx, kilitlenmiş bir iş emri üzerindeki işlemleri gerçekleştirir.
type orderTx struct {
	q     querier
	order workorder.WorkOrder
	saved *workorder.WorkOrder
}

var _ workorder.Tx = (*orderTx)(nil)

// finalOrder, işlem sonunda iş emrinin son hâlini döner.
func (t *orderTx) finalOrder() workorder.WorkOrder {
	if t.saved != nil {
		return *t.saved
	}
	return t.order
}

func (t *orderTx) Order() workorder.WorkOrder { return t.order }

func (t *orderTx) BOM(ctx context.Context) ([]workorder.BOMLine, error) {
	const query = `
		SELECT material_id, qty_per_unit
		FROM bom_lines
		WHERE product_id = $1
		ORDER BY material_id`

	rows, err := t.q.Query(ctx, query, t.order.ProductID)
	if err != nil {
		return nil, fmt.Errorf("reçete sorgulanamadı: %w", err)
	}
	defer rows.Close()

	var lines []workorder.BOMLine
	for rows.Next() {
		var line workorder.BOMLine
		if err := rows.Scan(&line.MaterialID, &line.QtyPerUnit); err != nil {
			return nil, fmt.Errorf("reçete satırı okunamadı: %w", err)
		}
		lines = append(lines, line)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reçete okunamadı: %w", err)
	}
	return lines, nil
}

func (t *orderTx) Save(ctx context.Context, w workorder.WorkOrder) error {
	const query = `
		UPDATE work_orders
		SET status = $2, started_qty = $3, good_qty = $4, scrap_qty = $5,
		    rework_pending = $6, updated_at = now()
		WHERE id = $1`

	tag, err := t.q.Exec(ctx, query, t.order.ID, string(w.Status),
		w.StartedQty, w.GoodQty, w.ScrapQty, w.ReworkPending)
	if err != nil {
		return fmt.Errorf("iş emri kaydedilemedi: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return workorder.ErrOrderNotFound
	}
	t.saved = &w
	return nil
}

// Consume, malzemeleri stoktan düşer. Gereksinimler malzeme kimliğine göre sıralı gelir,
// böylece eşzamanlı işlemler stok satırlarını hep aynı sırayla kilitler (kilitlenme riski azalır).
//
// Düşüm koşullu tek bir UPDATE ile yapılır: stok yetmiyorsa satır güncellenmez.
// Önce okuyup sonra yazmadığımız için iki işlem aynı anda gelse bile stok eksiye düşemez.
func (t *orderTx) Consume(ctx context.Context, requirements []workorder.Requirement) error {
	const decrement = `
		UPDATE materials
		SET stock_qty = stock_qty - $1
		WHERE id = $2 AND stock_qty >= $1`
	const record = `
		INSERT INTO material_consumptions (work_order_id, material_id, quantity)
		VALUES ($1, $2, $3)
		ON CONFLICT (work_order_id, material_id)
		DO UPDATE SET quantity = material_consumptions.quantity + EXCLUDED.quantity`

	for _, requirement := range requirements {
		tag, err := t.q.Exec(ctx, decrement, requirement.Quantity, requirement.MaterialID)
		if err != nil {
			return fmt.Errorf("stok düşülemedi: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return fmt.Errorf("%w: malzeme %d için %d birim gerekli",
				workorder.ErrInsufficientStock, requirement.MaterialID, requirement.Quantity)
		}

		if _, err := t.q.Exec(ctx, record, t.order.ID, requirement.MaterialID, requirement.Quantity); err != nil {
			return fmt.Errorf("tüketim kaydedilemedi: %w", err)
		}
	}
	return nil
}

func (t *orderTx) AddRecord(ctx context.Context, record workorder.Record) error {
	const query = `
		INSERT INTO production_records (work_order_id, kind, good_qty, scrap_qty, rework_qty, created_by)
		VALUES ($1, $2, $3, $4, $5, $6)`

	_, err := t.q.Exec(ctx, query, t.order.ID, string(record.Kind),
		record.Good, record.Scrap, record.Rework, record.UserID)
	if err != nil {
		return fmt.Errorf("üretim kaydı eklenemedi: %w", err)
	}
	return nil
}

func (t *orderTx) Consumed(ctx context.Context) (map[int64]int64, error) {
	const query = `SELECT material_id, quantity FROM material_consumptions WHERE work_order_id = $1`

	rows, err := t.q.Query(ctx, query, t.order.ID)
	if err != nil {
		return nil, fmt.Errorf("tüketim sorgulanamadı: %w", err)
	}
	defer rows.Close()

	consumed := make(map[int64]int64)
	for rows.Next() {
		var materialID, quantity int64
		if err := rows.Scan(&materialID, &quantity); err != nil {
			return nil, fmt.Errorf("tüketim okunamadı: %w", err)
		}
		consumed[materialID] = quantity
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("tüketim okunamadı: %w", err)
	}
	return consumed, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanOrder(row scanner) (workorder.WorkOrder, error) {
	var (
		order  workorder.WorkOrder
		status string
	)
	err := row.Scan(&order.ID, &order.ProductID, &order.PlannedQty, &status,
		&order.StartedQty, &order.GoodQty, &order.ScrapQty, &order.ReworkPending)
	if errors.Is(err, pgx.ErrNoRows) {
		return workorder.WorkOrder{}, workorder.ErrOrderNotFound
	}
	if err != nil {
		return workorder.WorkOrder{}, fmt.Errorf("iş emri okunamadı: %w", err)
	}
	order.Status = workorder.Status(status)
	return order, nil
}
