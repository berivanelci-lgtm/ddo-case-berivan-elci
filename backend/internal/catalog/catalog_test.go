package catalog

import (
	"errors"
	"strings"
	"testing"
)

func validBOM() []BOMItem {
	return []BOMItem{{MaterialID: 1, QtyPerUnit: 5}, {MaterialID: 2, QtyPerUnit: 3}}
}

func manyLines(count int) []BOMItem {
	lines := make([]BOMItem, count)
	for i := range lines {
		lines[i] = BOMItem{MaterialID: int64(i + 1), QtyPerUnit: 1}
	}
	return lines
}

func TestValidateMaterial_TrimsAndAccepts(t *testing.T) {
	code, name, err := ValidateMaterial("  CLK-01 ", " Çelik plaka ", 100)
	if err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}

	if code != "CLK-01" || name != "Çelik plaka" {
		t.Errorf("boşluklar temizlenmeli, gelen %q ve %q", code, name)
	}
}

func TestValidateMaterial_AllowsZeroStock(t *testing.T) {
	if _, _, err := ValidateMaterial("A", "B", 0); err != nil {
		t.Fatalf("sıfır stok geçerli olmalı: %v", err)
	}
}

func TestValidateMaterial_CountsCharactersNotBytes(t *testing.T) {
	if _, _, err := ValidateMaterial("A", strings.Repeat("ç", 100), 1); err != nil {
		t.Errorf("100 karakterlik ad kabul edilmeli: %v", err)
	}
	if _, _, err := ValidateMaterial("A", strings.Repeat("ç", 101), 1); !errors.Is(err, ErrInvalid) {
		t.Errorf("101 karakterlik ad reddedilmeli, gelen %v", err)
	}
}

func TestValidateMaterial_Rejects(t *testing.T) {
	cases := []struct {
		name         string
		code         string
		materialName string
		stock        int64
	}{
		{"boş kod", "", "Ad", 1},
		{"boşluktan oluşan kod", "   ", "Ad", 1},
		{"çok uzun kod", strings.Repeat("a", 41), "Ad", 1},
		{"boş ad", "K", "", 1},
		{"çok uzun ad", "K", strings.Repeat("a", 101), 1},
		{"negatif stok", "K", "Ad", -1},
		{"sınırı aşan stok", "K", "Ad", MaxQuantity + 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := ValidateMaterial(tc.code, tc.materialName, tc.stock); !errors.Is(err, ErrInvalid) {
				t.Fatalf("ErrInvalid beklenirdi, gelen %v", err)
			}
		})
	}
}

func TestValidateProduct_Accepts(t *testing.T) {
	code, name, err := ValidateProduct(" P-1 ", " Fren diski ", validBOM())
	if err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}

	if code != "P-1" || name != "Fren diski" {
		t.Errorf("boşluklar temizlenmeli, gelen %q ve %q", code, name)
	}
}

func TestValidateProduct_AcceptsMaximumBOMSize(t *testing.T) {
	if _, _, err := ValidateProduct("P", "Ürün", manyLines(maxBOMLines)); err != nil {
		t.Fatalf("%d satırlık reçete kabul edilmeli: %v", maxBOMLines, err)
	}
}

func TestValidateProduct_Rejects(t *testing.T) {
	cases := []struct {
		name string
		code string
		bom  []BOMItem
	}{
		{"boş kod", "", validBOM()},
		{"reçete yok", "P", nil},
		{"boş reçete", "P", []BOMItem{}},
		{"çok fazla satır", "P", manyLines(maxBOMLines + 1)},
		{"sıfır malzeme kimliği", "P", []BOMItem{{MaterialID: 0, QtyPerUnit: 1}}},
		{"sıfır miktar", "P", []BOMItem{{MaterialID: 1, QtyPerUnit: 0}}},
		{"negatif miktar", "P", []BOMItem{{MaterialID: 1, QtyPerUnit: -2}}},
		{"sınırı aşan miktar", "P", []BOMItem{{MaterialID: 1, QtyPerUnit: MaxQuantity + 1}}},
		{"tekrarlı malzeme", "P", []BOMItem{{MaterialID: 1, QtyPerUnit: 1}, {MaterialID: 1, QtyPerUnit: 2}}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := ValidateProduct(tc.code, "Ürün", tc.bom); !errors.Is(err, ErrInvalid) {
				t.Fatalf("ErrInvalid beklenirdi, gelen %v", err)
			}
		})
	}
}
