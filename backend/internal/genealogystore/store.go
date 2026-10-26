// Package genealogystore, soy ağacı sorgularını PostgreSQL'de recursive CTE ile yapar.
package genealogystore

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/genealogy"
)

// Store, genealogy.Repository arayüzünü PostgreSQL ile gerçekler.
type Store struct {
	pool *pgxpool.Pool
}

var _ genealogy.Repository = (*Store)(nil)

// New, yeni bir Store oluşturur.
func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// Geri izleme: partiyi üretmek için kullanılan partiler, onların girdileri...
// UNION aynı (parti, seviye) satırlarını tekrarlamaz; seviye sınırı döngüye karşı korur.
const backwardQuery = `
	WITH RECURSIVE up(lot_id, depth) AS (
		SELECT input_lot_id, 1 FROM lot_inputs WHERE output_lot_id = $1
		UNION
		SELECT li.input_lot_id, up.depth + 1
		FROM lot_inputs li
		JOIN up ON li.output_lot_id = up.lot_id
		WHERE up.depth < $2::int
	)
	SELECT l.id, l.code, l.kind, min(up.depth)
	FROM up
	JOIN lots l ON l.id = up.lot_id
	GROUP BY l.id, l.code, l.kind
	ORDER BY 4, 2`

// İleri izleme: partinin kullanıldığı partiler, onların kullanıldığı partiler...
const forwardQuery = `
	WITH RECURSIVE down(lot_id, depth) AS (
		SELECT output_lot_id, 1 FROM lot_inputs WHERE input_lot_id = $1
		UNION
		SELECT li.output_lot_id, down.depth + 1
		FROM lot_inputs li
		JOIN down ON li.input_lot_id = down.lot_id
		WHERE down.depth < $2::int
	)
	SELECT l.id, l.code, l.kind, min(down.depth)
	FROM down
	JOIN lots l ON l.id = down.lot_id
	GROUP BY l.id, l.code, l.kind
	ORDER BY 4, 2`

// Etkilenen sevkiyatlar: partinin kendisi ve tüm alt partileri hangi sevkiyatlarda?
const shipmentsQuery = `
	WITH RECURSIVE down(lot_id, depth) AS (
		SELECT $1::bigint, 0
		UNION
		SELECT li.output_lot_id, down.depth + 1
		FROM lot_inputs li
		JOIN down ON li.input_lot_id = down.lot_id
		WHERE down.depth < $2::int
	)
	SELECT s.id, s.code, s.customer, s.shipped_at, count(DISTINCT si.lot_id)
	FROM (SELECT DISTINCT lot_id FROM down) d
	JOIN shipment_items si ON si.lot_id = d.lot_id
	JOIN shipments s ON s.id = si.shipment_id
	GROUP BY s.id, s.code, s.customer, s.shipped_at
	ORDER BY s.code`

// FindLot, partiyi koduyla bulur.
func (s *Store) FindLot(ctx context.Context, code string) (genealogy.Lot, error) {
	var (
		lot  genealogy.Lot
		kind string
	)
	err := s.pool.QueryRow(ctx, `SELECT id, code, kind FROM lots WHERE code = $1`, code).
		Scan(&lot.ID, &lot.Code, &kind)
	if errors.Is(err, pgx.ErrNoRows) {
		return genealogy.Lot{}, genealogy.ErrLotNotFound
	}
	if err != nil {
		return genealogy.Lot{}, fmt.Errorf("parti sorgulanamadı: %w", err)
	}
	lot.Kind = genealogy.Kind(kind)
	return lot, nil
}

// Backward, geri izleme sonucunu döner.
func (s *Store) Backward(ctx context.Context, lotID int64, maxDepth int) ([]genealogy.Node, error) {
	return s.queryNodes(ctx, backwardQuery, lotID, maxDepth)
}

// Forward, ileri izleme sonucunu döner.
func (s *Store) Forward(ctx context.Context, lotID int64, maxDepth int) ([]genealogy.Node, error) {
	return s.queryNodes(ctx, forwardQuery, lotID, maxDepth)
}

// Shipments, partiden etkilenen sevkiyatları döner.
func (s *Store) Shipments(ctx context.Context, lotID int64, maxDepth int) ([]genealogy.Shipment, error) {
	rows, err := s.pool.Query(ctx, shipmentsQuery, lotID, maxDepth)
	if err != nil {
		return nil, fmt.Errorf("sevkiyatlar sorgulanamadı: %w", err)
	}
	defer rows.Close()

	shipments := make([]genealogy.Shipment, 0)
	for rows.Next() {
		var shipment genealogy.Shipment
		if err := rows.Scan(&shipment.ID, &shipment.Code, &shipment.Customer, &shipment.ShippedAt, &shipment.Units); err != nil {
			return nil, fmt.Errorf("sevkiyat okunamadı: %w", err)
		}
		shipments = append(shipments, shipment)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sevkiyatlar okunamadı: %w", err)
	}
	return shipments, nil
}

func (s *Store) queryNodes(ctx context.Context, query string, lotID int64, maxDepth int) ([]genealogy.Node, error) {
	rows, err := s.pool.Query(ctx, query, lotID, maxDepth)
	if err != nil {
		return nil, fmt.Errorf("soy ağacı sorgulanamadı: %w", err)
	}
	defer rows.Close()

	nodes := make([]genealogy.Node, 0)
	for rows.Next() {
		var (
			node genealogy.Node
			kind string
		)
		if err := rows.Scan(&node.ID, &node.Code, &kind, &node.Depth); err != nil {
			return nil, fmt.Errorf("parti okunamadı: %w", err)
		}
		node.Kind = genealogy.Kind(kind)
		nodes = append(nodes, node)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("soy ağacı okunamadı: %w", err)
	}
	return nodes, nil
}
