// Package catalogstore, malzeme ve ürün kataloğunu PostgreSQL'de saklar.
package catalogstore

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/audit"
	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/catalog"
)

const (
	uniqueViolation     = "23505"
	foreignKeyViolation = "23503"
)

// Store, catalog.Repository arayüzünü PostgreSQL ile gerçekler.
type Store struct {
	pool *pgxpool.Pool
}

var _ catalog.Repository = (*Store)(nil)

// New, yeni bir Store oluşturur.
func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// CreateMaterial, başlangıç stoğuyla yeni bir malzeme ekler.
func (s *Store) CreateMaterial(ctx context.Context, code, name string, stockQty int64) (catalog.Material, error) {
	tx, err := audit.Begin(ctx, s.pool)
	if err != nil {
		return catalog.Material{}, fmt.Errorf("işlem başlatılamadı: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const query = `
		INSERT INTO materials (code, name, stock_qty)
		VALUES ($1, $2, $3)
		RETURNING id`

	material := catalog.Material{Code: code, Name: name, StockQty: stockQty}
	err = tx.QueryRow(ctx, query, code, name, stockQty).Scan(&material.ID)
	if hasCode(err, uniqueViolation) {
		return catalog.Material{}, catalog.ErrCodeExists
	}
	if err != nil {
		return catalog.Material{}, fmt.Errorf("malzeme oluşturulamadı: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return catalog.Material{}, fmt.Errorf("işlem onaylanamadı: %w", err)
	}
	return material, nil
}

// ListMaterials, malzemeleri güncel stoklarıyla kimlik sırasında getirir.
func (s *Store) ListMaterials(ctx context.Context) ([]catalog.Material, error) {
	const query = `SELECT id, code, name, stock_qty FROM materials ORDER BY id`

	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("malzemeler sorgulanamadı: %w", err)
	}
	defer rows.Close()

	var materials []catalog.Material
	for rows.Next() {
		var material catalog.Material
		if err := rows.Scan(&material.ID, &material.Code, &material.Name, &material.StockQty); err != nil {
			return nil, fmt.Errorf("malzeme okunamadı: %w", err)
		}
		materials = append(materials, material)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("malzemeler okunamadı: %w", err)
	}
	return materials, nil
}

// CreateProduct, ürünü ve reçetesini tek bir işlemde ekler. Reçetede olmayan bir malzeme
// varsa hiçbir şey eklenmez.
func (s *Store) CreateProduct(ctx context.Context, code, name string, bom []catalog.BOMItem) (catalog.Product, error) {
	tx, err := audit.Begin(ctx, s.pool)
	if err != nil {
		return catalog.Product{}, fmt.Errorf("işlem başlatılamadı: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	product := catalog.Product{Code: code, Name: name, BOM: bom}
	err = tx.QueryRow(ctx,
		`INSERT INTO products (code, name) VALUES ($1, $2) RETURNING id`, code, name).Scan(&product.ID)
	if hasCode(err, uniqueViolation) {
		return catalog.Product{}, catalog.ErrCodeExists
	}
	if err != nil {
		return catalog.Product{}, fmt.Errorf("ürün oluşturulamadı: %w", err)
	}

	for _, item := range bom {
		_, err := tx.Exec(ctx,
			`INSERT INTO bom_lines (product_id, material_id, qty_per_unit) VALUES ($1, $2, $3)`,
			product.ID, item.MaterialID, item.QtyPerUnit)
		if hasCode(err, foreignKeyViolation) {
			return catalog.Product{}, fmt.Errorf("%w: %d", catalog.ErrMaterialNotFound, item.MaterialID)
		}
		if err != nil {
			return catalog.Product{}, fmt.Errorf("reçete satırı eklenemedi: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return catalog.Product{}, fmt.Errorf("işlem onaylanamadı: %w", err)
	}
	return product, nil
}

// ListProducts, ürünleri reçeteleriyle birlikte kimlik sırasında getirir.
func (s *Store) ListProducts(ctx context.Context) ([]catalog.Product, error) {
	const query = `
		SELECT p.id, p.code, p.name, b.material_id, b.qty_per_unit
		FROM products p
		LEFT JOIN bom_lines b ON b.product_id = p.id
		ORDER BY p.id, b.material_id`

	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("ürünler sorgulanamadı: %w", err)
	}
	defer rows.Close()

	var products []catalog.Product
	for rows.Next() {
		var (
			id         int64
			code, name string
			materialID *int64
			qtyPerUnit *int64
		)
		if err := rows.Scan(&id, &code, &name, &materialID, &qtyPerUnit); err != nil {
			return nil, fmt.Errorf("ürün okunamadı: %w", err)
		}

		if len(products) == 0 || products[len(products)-1].ID != id {
			products = append(products, catalog.Product{ID: id, Code: code, Name: name})
		}
		if materialID != nil && qtyPerUnit != nil {
			last := &products[len(products)-1]
			last.BOM = append(last.BOM, catalog.BOMItem{MaterialID: *materialID, QtyPerUnit: *qtyPerUnit})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ürünler okunamadı: %w", err)
	}
	return products, nil
}

func hasCode(err error, code string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == code
}
