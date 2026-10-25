package workorder

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
)

var (
	// ErrEmptyBOM, ürünün reçetesi boşsa döner.
	ErrEmptyBOM = errors.New("ürün reçetesi boş")
	// ErrInvalidBOM, reçete satırları geçersizse döner.
	ErrInvalidBOM = errors.New("ürün reçetesi geçersiz")
)

// BOMLine, ürün reçetesinin (BOM) bir satırıdır: bir ürün için gereken malzeme miktarı.
type BOMLine struct {
	MaterialID int64
	QtyPerUnit int64
}

// Requirement, bir malzemeden gereken toplam miktardır.
type Requirement struct {
	MaterialID int64
	Quantity   int64
}

// Requirements, verilen adette ürün için gereken malzemeleri hesaplar.
// Sonuç malzeme kimliğine göre sıralıdır. Veritabanında stok satırlarını hep
// aynı sırayla kilitlemek için bu sıra önemlidir (kilitlenme riskini azaltır).
func Requirements(bom []BOMLine, units int64) ([]Requirement, error) {
	if units <= 0 {
		return nil, fmt.Errorf("%w: ürün adedi sıfırdan büyük olmalı", ErrInvalidQuantity)
	}
	if len(bom) == 0 {
		return nil, ErrEmptyBOM
	}

	seen := make(map[int64]bool, len(bom))
	requirements := make([]Requirement, 0, len(bom))
	for _, line := range bom {
		if line.QtyPerUnit <= 0 {
			return nil, fmt.Errorf("%w: malzeme %d için birim miktar sıfırdan büyük olmalı", ErrInvalidBOM, line.MaterialID)
		}
		if seen[line.MaterialID] {
			return nil, fmt.Errorf("%w: malzeme %d reçetede birden fazla kez geçiyor", ErrInvalidBOM, line.MaterialID)
		}
		seen[line.MaterialID] = true

		if line.QtyPerUnit > math.MaxInt64/units {
			return nil, fmt.Errorf("%w: malzeme %d için gereken miktar çok büyük", ErrInvalidQuantity, line.MaterialID)
		}
		requirements = append(requirements, Requirement{
			MaterialID: line.MaterialID,
			Quantity:   line.QtyPerUnit * units,
		})
	}

	sort.Slice(requirements, func(i, j int) bool {
		return requirements[i].MaterialID < requirements[j].MaterialID
	})
	return requirements, nil
}

// Reconcile, iş emri için gerçekte tüketilen malzemenin beklenenle tuttuğunu denetler:
// tüketim = başlayan ürün adedi × reçete miktarı. Tutmayan her malzeme hata mesajında listelenir.
func Reconcile(w WorkOrder, bom []BOMLine, consumed map[int64]int64) error {
	if err := w.CheckInvariants(); err != nil {
		return err
	}

	expected := make(map[int64]int64)
	if w.StartedQty > 0 {
		requirements, err := Requirements(bom, w.StartedQty)
		if err != nil {
			return err
		}
		for _, requirement := range requirements {
			expected[requirement.MaterialID] = requirement.Quantity
		}
	}

	ids := make(map[int64]struct{})
	for id := range expected {
		ids[id] = struct{}{}
	}
	for id, quantity := range consumed {
		if quantity != 0 {
			ids[id] = struct{}{}
		}
	}
	sorted := make([]int64, 0, len(ids))
	for id := range ids {
		sorted = append(sorted, id)
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	var problems []string
	for _, id := range sorted {
		if got := consumed[id]; got != expected[id] {
			problems = append(problems, fmt.Sprintf("malzeme %d: beklenen %d, gelen %d", id, expected[id], got))
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("%w: %s", ErrInconsistent, strings.Join(problems, "; "))
	}
	return nil
}
