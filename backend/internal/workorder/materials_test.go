package workorder

import (
	"errors"
	"math"
	"strings"
	"testing"
)

func testBOM() []BOMLine {
	return []BOMLine{
		{MaterialID: 1, QtyPerUnit: 5},
		{MaterialID: 2, QtyPerUnit: 3},
	}
}

// producedOrder, 10 ürünün başladığı bir iş emri döner (8 sağlam, 2 fire).
func producedOrder(t *testing.T) WorkOrder {
	t.Helper()
	return mustProduce(t, mustRelease(t, newOrder(t, 20)), 8, 2, 0)
}

func TestRequirements_ScalesBOMAndSortsByMaterial(t *testing.T) {
	unsorted := []BOMLine{
		{MaterialID: 2, QtyPerUnit: 3},
		{MaterialID: 1, QtyPerUnit: 5},
	}

	got, err := Requirements(unsorted, 4)
	if err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}

	want := []Requirement{
		{MaterialID: 1, Quantity: 20},
		{MaterialID: 2, Quantity: 12},
	}
	if len(got) != len(want) {
		t.Fatalf("%d satır beklenirdi, gelen %d", len(want), len(got))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%d. satır: beklenen %+v, gelen %+v", i+1, want[i], got[i])
		}
	}
}

func TestRequirements_RejectsInvalidInput(t *testing.T) {
	cases := []struct {
		name  string
		bom   []BOMLine
		units int64
		want  error
	}{
		{"birim sıfır", testBOM(), 0, ErrInvalidQuantity},
		{"birim negatif", testBOM(), -3, ErrInvalidQuantity},
		{"boş reçete", nil, 5, ErrEmptyBOM},
		{"sıfır miktar", []BOMLine{{MaterialID: 1, QtyPerUnit: 0}}, 5, ErrInvalidBOM},
		{"negatif miktar", []BOMLine{{MaterialID: 1, QtyPerUnit: -2}}, 5, ErrInvalidBOM},
		{
			"tekrarlı malzeme",
			[]BOMLine{{MaterialID: 1, QtyPerUnit: 1}, {MaterialID: 1, QtyPerUnit: 2}},
			5,
			ErrInvalidBOM,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Requirements(tc.bom, tc.units); !errors.Is(err, tc.want) {
				t.Fatalf("%v beklenirdi, gelen %v", tc.want, err)
			}
		})
	}
}

func TestRequirements_RejectsOverflow(t *testing.T) {
	bom := []BOMLine{{MaterialID: 1, QtyPerUnit: math.MaxInt64 / 2}}

	if _, err := Requirements(bom, 3); !errors.Is(err, ErrInvalidQuantity) {
		t.Fatalf("taşma için ErrInvalidQuantity beklenirdi, gelen %v", err)
	}
}

func TestReconcile_AcceptsMatchingConsumption(t *testing.T) {
	// 10 ürün başladı: malzeme 1 için 10×5=50, malzeme 2 için 10×3=30.
	consumed := map[int64]int64{1: 50, 2: 30}

	if err := Reconcile(producedOrder(t), testBOM(), consumed); err != nil {
		t.Fatalf("tutarlı tüketim kabul edilmeli: %v", err)
	}
}

func TestReconcile_DetectsMissingConsumption(t *testing.T) {
	consumed := map[int64]int64{1: 50, 2: 29}

	err := Reconcile(producedOrder(t), testBOM(), consumed)

	if !errors.Is(err, ErrInconsistent) {
		t.Fatalf("ErrInconsistent beklenirdi, gelen %v", err)
	}
	if !strings.Contains(err.Error(), "malzeme 2") {
		t.Errorf("hata mesajı sorunlu malzemeyi göstermeli: %v", err)
	}
}

func TestReconcile_DetectsUnexpectedMaterial(t *testing.T) {
	consumed := map[int64]int64{1: 50, 2: 30, 3: 1}

	if err := Reconcile(producedOrder(t), testBOM(), consumed); !errors.Is(err, ErrInconsistent) {
		t.Fatalf("reçetede olmayan malzeme için ErrInconsistent beklenirdi, gelen %v", err)
	}
}

func TestReconcile_IgnoresZeroEntries(t *testing.T) {
	consumed := map[int64]int64{1: 50, 2: 30, 3: 0}

	if err := Reconcile(producedOrder(t), testBOM(), consumed); err != nil {
		t.Fatalf("sıfır tüketim kaydı sorun olmamalı: %v", err)
	}
}

func TestReconcile_NoProduction(t *testing.T) {
	released := mustRelease(t, newOrder(t, 20))

	if err := Reconcile(released, testBOM(), map[int64]int64{}); err != nil {
		t.Errorf("üretim yokken tüketim yoksa tutarlı olmalı: %v", err)
	}
	if err := Reconcile(released, testBOM(), map[int64]int64{1: 1}); !errors.Is(err, ErrInconsistent) {
		t.Errorf("üretim yokken tüketim varsa ErrInconsistent beklenirdi, gelen %v", err)
	}
}

func TestReconcile_RejectsBrokenOrder(t *testing.T) {
	broken := WorkOrder{Status: StatusInProgress, StartedQty: 5, GoodQty: 1}

	if err := Reconcile(broken, testBOM(), map[int64]int64{}); !errors.Is(err, ErrInconsistent) {
		t.Fatalf("ErrInconsistent beklenirdi, gelen %v", err)
	}
}
