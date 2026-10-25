// Package catalog, malzemeleri, ürünleri ve ürün reçetelerini (BOM) tanımlar.
package catalog

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	maxCodeLength = 40
	maxNameLength = 100
	maxBOMLines   = 50

	// MaxQuantity, tek bir miktar alanı için kabul edilen en büyük değerdir.
	MaxQuantity int64 = 1_000_000_000
)

var (
	// ErrInvalid, girilen değerler geçersiz olduğunda döner.
	ErrInvalid = errors.New("geçersiz değer")
	// ErrCodeExists, aynı kodla bir kayıt zaten varsa döner.
	ErrCodeExists = errors.New("bu kodla bir kayıt zaten var")
	// ErrMaterialNotFound, reçetede olmayan bir malzeme kullanıldığında döner.
	ErrMaterialNotFound = errors.New("malzeme bulunamadı")
)

// Material, stoğu tutulan bir hammadde ya da parçadır.
type Material struct {
	ID       int64
	Code     string
	Name     string
	StockQty int64
}

// BOMItem, ürün reçetesinin bir satırıdır: bir ürün için gereken malzeme miktarı.
type BOMItem struct {
	MaterialID int64
	QtyPerUnit int64
}

// Product, reçetesiyle birlikte bir ürünü temsil eder.
type Product struct {
	ID   int64
	Code string
	Name string
	BOM  []BOMItem
}

// Repository, katalog verilerinin saklandığı yerdir.
type Repository interface {
	CreateMaterial(ctx context.Context, code, name string, stockQty int64) (Material, error)
	ListMaterials(ctx context.Context) ([]Material, error)
	CreateProduct(ctx context.Context, code, name string, bom []BOMItem) (Product, error)
	ListProducts(ctx context.Context) ([]Product, error)
}

// ValidateMaterial, malzeme bilgilerini doğrular ve temizlenmiş kod ile adı döner.
func ValidateMaterial(code, name string, stockQty int64) (string, string, error) {
	cleanCode, cleanName, err := cleanCodeAndName(code, name)
	if err != nil {
		return "", "", err
	}
	if stockQty < 0 || stockQty > MaxQuantity {
		return "", "", fmt.Errorf("%w: stok 0 ile %d arasında olmalı", ErrInvalid, MaxQuantity)
	}
	return cleanCode, cleanName, nil
}

// ValidateProduct, ürün ve reçetesini doğrular ve temizlenmiş kod ile adı döner.
func ValidateProduct(code, name string, bom []BOMItem) (string, string, error) {
	cleanCode, cleanName, err := cleanCodeAndName(code, name)
	if err != nil {
		return "", "", err
	}
	if len(bom) == 0 || len(bom) > maxBOMLines {
		return "", "", fmt.Errorf("%w: reçete 1 ile %d satır içermeli", ErrInvalid, maxBOMLines)
	}

	seen := make(map[int64]bool, len(bom))
	for _, item := range bom {
		if item.MaterialID <= 0 {
			return "", "", fmt.Errorf("%w: malzeme kimliği pozitif olmalı", ErrInvalid)
		}
		if item.QtyPerUnit <= 0 || item.QtyPerUnit > MaxQuantity {
			return "", "", fmt.Errorf("%w: birim miktar 1 ile %d arasında olmalı", ErrInvalid, MaxQuantity)
		}
		if seen[item.MaterialID] {
			return "", "", fmt.Errorf("%w: malzeme %d reçetede birden fazla kez geçiyor", ErrInvalid, item.MaterialID)
		}
		seen[item.MaterialID] = true
	}
	return cleanCode, cleanName, nil
}

func cleanCodeAndName(code, name string) (string, string, error) {
	cleanCode, err := cleanText("kod", code, maxCodeLength)
	if err != nil {
		return "", "", err
	}
	cleanName, err := cleanText("ad", name, maxNameLength)
	if err != nil {
		return "", "", err
	}
	return cleanCode, cleanName, nil
}

func cleanText(field, value string, maxLength int) (string, error) {
	cleaned := strings.TrimSpace(value)
	if cleaned == "" || utf8.RuneCountInString(cleaned) > maxLength {
		return "", fmt.Errorf("%w: %s 1 ile %d karakter arasında olmalı", ErrInvalid, field, maxLength)
	}
	return cleaned, nil
}
