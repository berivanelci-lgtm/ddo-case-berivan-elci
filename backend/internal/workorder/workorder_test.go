package workorder

import (
	"errors"
	"testing"
)

func newOrder(t *testing.T, planned int64) WorkOrder {
	t.Helper()
	w, err := New(7, planned)
	if err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}
	return w
}

func mustRelease(t *testing.T, w WorkOrder) WorkOrder {
	t.Helper()
	released, err := w.Release()
	if err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}
	return released
}

func mustProduce(t *testing.T, w WorkOrder, good, scrap, rework int64) WorkOrder {
	t.Helper()
	updated, err := w.RecordProduction(good, scrap, rework)
	if err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}
	return updated
}

func mustRework(t *testing.T, w WorkOrder, good, scrap int64) WorkOrder {
	t.Helper()
	updated, err := w.RecordRework(good, scrap)
	if err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}
	return updated
}

func mustClose(t *testing.T, w WorkOrder, allowPartial bool) WorkOrder {
	t.Helper()
	closed, err := w.Close(allowPartial)
	if err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}
	return closed
}

func TestNew_RejectsInvalidPlannedQuantity(t *testing.T) {
	for _, planned := range []int64{0, -5, MaxQuantity + 1} {
		if _, err := New(7, planned); !errors.Is(err, ErrInvalidQuantity) {
			t.Errorf("%d için ErrInvalidQuantity beklenirdi, gelen %v", planned, err)
		}
	}
}

func TestNew_StartsPlanned(t *testing.T) {
	w := newOrder(t, 10)

	if w.Status != StatusPlanned || w.ProductID != 7 || w.PlannedQty != 10 || w.StartedQty != 0 {
		t.Fatalf("beklenmeyen başlangıç durumu: %+v", w)
	}
}

func TestRelease_OnlyFromPlanned(t *testing.T) {
	released := mustRelease(t, newOrder(t, 10))

	if released.Status != StatusReleased {
		t.Fatalf("released beklenirdi, gelen %s", released.Status)
	}
	if _, err := released.Release(); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("ikinci serbest bırakma ErrInvalidTransition vermeli, gelen %v", err)
	}
}

func TestCancel_AllowedBeforeProductionStarts(t *testing.T) {
	orders := map[string]WorkOrder{
		"planlandı": newOrder(t, 10),
		"serbest":   mustRelease(t, newOrder(t, 10)),
	}

	for name, w := range orders {
		t.Run(name, func(t *testing.T) {
			cancelled, err := w.Cancel()
			if err != nil {
				t.Fatalf("beklenmeyen hata: %v", err)
			}
			if cancelled.Status != StatusCancelled {
				t.Fatalf("cancelled beklenirdi, gelen %s", cancelled.Status)
			}
		})
	}
}

func TestCancel_RejectedOnceProductionStarted(t *testing.T) {
	started := mustProduce(t, mustRelease(t, newOrder(t, 10)), 3, 0, 0)
	completed := mustClose(t, mustProduce(t, mustRelease(t, newOrder(t, 10)), 10, 0, 0), false)

	for name, w := range map[string]WorkOrder{"üretimde": started, "tamamlandı": completed} {
		t.Run(name, func(t *testing.T) {
			if _, err := w.Cancel(); !errors.Is(err, ErrInvalidTransition) {
				t.Fatalf("ErrInvalidTransition beklenirdi, gelen %v", err)
			}
		})
	}
}

func TestRecordProduction_RejectedOutsideActiveStates(t *testing.T) {
	cancelled, err := newOrder(t, 10).Cancel()
	if err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}
	completed := mustClose(t, mustProduce(t, mustRelease(t, newOrder(t, 10)), 10, 0, 0), false)

	orders := map[string]WorkOrder{
		"planlandı":  newOrder(t, 10),
		"tamamlandı": completed,
		"iptal":      cancelled,
	}
	for name, w := range orders {
		t.Run(name, func(t *testing.T) {
			if _, err := w.RecordProduction(1, 0, 0); !errors.Is(err, ErrInvalidTransition) {
				t.Fatalf("ErrInvalidTransition beklenirdi, gelen %v", err)
			}
		})
	}
}

func TestRecordProduction_StartsWork(t *testing.T) {
	got := mustProduce(t, mustRelease(t, newOrder(t, 20)), 4, 1, 0)

	if got.Status != StatusInProgress {
		t.Errorf("in_progress beklenirdi, gelen %s", got.Status)
	}
	if got.StartedQty != 5 || got.GoodQty != 4 || got.ScrapQty != 1 || got.ReworkPending != 0 {
		t.Errorf("beklenmeyen sayaçlar: %+v", got)
	}
}

func TestRecordProduction_DoesNotChangeOriginal(t *testing.T) {
	released := mustRelease(t, newOrder(t, 20))

	_ = mustProduce(t, released, 4, 1, 0)

	if released.Status != StatusReleased || released.StartedQty != 0 {
		t.Fatalf("orijinal iş emri değişmemeli: %+v", released)
	}
}

func TestRecordProduction_RejectsInvalidQuantities(t *testing.T) {
	cases := []struct {
		name                string
		good, scrap, rework int64
	}{
		{"negatif sağlam", -1, 0, 0},
		{"negatif fire", 0, -1, 0},
		{"negatif yeniden işlem", 0, 0, -1},
		{"hepsi sıfır", 0, 0, 0},
		{"sınırı aşan miktar", MaxQuantity + 1, 0, 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := mustRelease(t, newOrder(t, 20))

			if _, err := w.RecordProduction(tc.good, tc.scrap, tc.rework); !errors.Is(err, ErrInvalidQuantity) {
				t.Fatalf("ErrInvalidQuantity beklenirdi, gelen %v", err)
			}
		})
	}
}

func TestRecordProduction_ScrapDoesNotCountAgainstPlan(t *testing.T) {
	got := mustProduce(t, mustRelease(t, newOrder(t, 10)), 10, 5, 0)

	if got.StartedQty != 15 || got.GoodQty != 10 || got.ScrapQty != 5 {
		t.Fatalf("fire plana sayılmamalı: %+v", got)
	}
}

func TestRecordProduction_CannotExceedPlan(t *testing.T) {
	t.Run("sağlam ürünle", func(t *testing.T) {
		full := mustProduce(t, mustRelease(t, newOrder(t, 10)), 10, 0, 0)

		if _, err := full.RecordProduction(1, 0, 0); !errors.Is(err, ErrExceedsPlan) {
			t.Fatalf("ErrExceedsPlan beklenirdi, gelen %v", err)
		}
	})

	t.Run("yeniden işlemdekiler de sayılır", func(t *testing.T) {
		mixed := mustProduce(t, mustRelease(t, newOrder(t, 10)), 6, 0, 4)

		if _, err := mixed.RecordProduction(1, 0, 0); !errors.Is(err, ErrExceedsPlan) {
			t.Fatalf("sağlam için ErrExceedsPlan beklenirdi, gelen %v", err)
		}
		if _, err := mixed.RecordProduction(0, 0, 1); !errors.Is(err, ErrExceedsPlan) {
			t.Fatalf("yeniden işlem için ErrExceedsPlan beklenirdi, gelen %v", err)
		}
	})
}

func TestRecordRework_ResolvesPendingUnits(t *testing.T) {
	w := mustProduce(t, mustRelease(t, newOrder(t, 10)), 6, 0, 4)

	got := mustRework(t, w, 3, 1)

	if got.GoodQty != 9 || got.ScrapQty != 1 || got.ReworkPending != 0 || got.StartedQty != 10 {
		t.Fatalf("beklenmeyen sayaçlar: %+v", got)
	}
	if err := got.CheckInvariants(); err != nil {
		t.Fatalf("sayaçlar tutmalı: %v", err)
	}
}

func TestRecordRework_CannotResolveMoreThanPending(t *testing.T) {
	w := mustProduce(t, mustRelease(t, newOrder(t, 10)), 6, 0, 2)

	if _, err := w.RecordRework(2, 1); !errors.Is(err, ErrInvalidQuantity) {
		t.Fatalf("ErrInvalidQuantity beklenirdi, gelen %v", err)
	}
}

func TestRecordRework_RejectsInvalidQuantities(t *testing.T) {
	cases := []struct {
		name        string
		good, scrap int64
	}{
		{"ikisi de sıfır", 0, 0},
		{"negatif sağlam", -1, 1},
		{"negatif fire", 1, -1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := mustProduce(t, mustRelease(t, newOrder(t, 10)), 6, 0, 4)

			if _, err := w.RecordRework(tc.good, tc.scrap); !errors.Is(err, ErrInvalidQuantity) {
				t.Fatalf("ErrInvalidQuantity beklenirdi, gelen %v", err)
			}
		})
	}
}

func TestRecordRework_RequiresInProgress(t *testing.T) {
	released := mustRelease(t, newOrder(t, 10))

	if _, err := released.RecordRework(1, 0); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("ErrInvalidTransition beklenirdi, gelen %v", err)
	}
}

func TestClose_RejectedWhileReworkPending(t *testing.T) {
	w := mustProduce(t, mustRelease(t, newOrder(t, 10)), 6, 0, 4)

	if _, err := w.Close(true); !errors.Is(err, ErrReworkPending) {
		t.Fatalf("kısmi onay olsa bile ErrReworkPending beklenirdi, gelen %v", err)
	}
}

func TestClose_PartialRequiresExplicitApproval(t *testing.T) {
	w := mustProduce(t, mustRelease(t, newOrder(t, 10)), 6, 0, 0)

	if _, err := w.Close(false); !errors.Is(err, ErrPartialNotAllowed) {
		t.Fatalf("ErrPartialNotAllowed beklenirdi, gelen %v", err)
	}
}

func TestClose_PartialCompletionAllowed(t *testing.T) {
	w := mustProduce(t, mustRelease(t, newOrder(t, 10)), 6, 2, 0)

	closed := mustClose(t, w, true)

	if closed.Status != StatusCompleted || closed.GoodQty != 6 || closed.ScrapQty != 2 {
		t.Fatalf("beklenmeyen kapanış durumu: %+v", closed)
	}
}

func TestClose_FullCompletion(t *testing.T) {
	w := mustProduce(t, mustRelease(t, newOrder(t, 10)), 10, 1, 0)

	closed := mustClose(t, w, false)

	if closed.Status != StatusCompleted {
		t.Fatalf("completed beklenirdi, gelen %s", closed.Status)
	}
}

func TestClose_RequiresInProgress(t *testing.T) {
	for name, w := range map[string]WorkOrder{
		"planlandı": newOrder(t, 10),
		"serbest":   mustRelease(t, newOrder(t, 10)),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := w.Close(true); !errors.Is(err, ErrInvalidTransition) {
				t.Fatalf("ErrInvalidTransition beklenirdi, gelen %v", err)
			}
		})
	}
}

func TestCompletedOrder_AcceptsNoMoreRecords(t *testing.T) {
	completed := mustClose(t, mustProduce(t, mustRelease(t, newOrder(t, 10)), 10, 0, 0), false)

	if _, err := completed.RecordProduction(1, 0, 0); !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("üretim kaydı: ErrInvalidTransition beklenirdi, gelen %v", err)
	}
	if _, err := completed.RecordRework(1, 0); !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("yeniden işlem: ErrInvalidTransition beklenirdi, gelen %v", err)
	}
	if _, err := completed.Release(); !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("serbest bırakma: ErrInvalidTransition beklenirdi, gelen %v", err)
	}
	if _, err := completed.Close(true); !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("kapatma: ErrInvalidTransition beklenirdi, gelen %v", err)
	}
}

// Rastgele işlem dizisinde değişmez kuralların bozulmadığını doğrular.
// Aynı tohum her seferinde aynı diziyi üretir.
func TestInvariants_HoldAcrossRandomOperations(t *testing.T) {
	state := uint32(2026)
	next := func(n int64) int64 {
		state = state*1664525 + 1013904223
		return int64(state>>8) % n
	}

	w := mustRelease(t, newOrder(t, 200))
	for i := 0; i < 500; i++ {
		var (
			updated WorkOrder
			err     error
		)
		if next(2) == 0 {
			updated, err = w.RecordProduction(next(8), next(3), next(4))
		} else {
			updated, err = w.RecordRework(next(4), next(2))
		}
		if err != nil {
			continue // reddedilen işlem durumu değiştirmemeli
		}

		w = updated
		if err := w.CheckInvariants(); err != nil {
			t.Fatalf("%d. işlemden sonra sayaçlar tutmuyor: %v (%+v)", i, err, w)
		}
		if w.GoodQty+w.ReworkPending > w.PlannedQty {
			t.Fatalf("%d. işlemden sonra plan aşıldı: %+v", i, w)
		}
	}
	if w.StartedQty == 0 {
		t.Fatal("hiç işlem kabul edilmedi, test anlamsız")
	}
}

func TestCheckInvariants_DetectsInconsistency(t *testing.T) {
	broken := map[string]WorkOrder{
		"negatif miktar":  {StartedQty: -1, GoodQty: -1},
		"toplam tutmuyor": {StartedQty: 5, GoodQty: 3, ScrapQty: 1},
	}
	for name, w := range broken {
		t.Run(name, func(t *testing.T) {
			if err := w.CheckInvariants(); !errors.Is(err, ErrInconsistent) {
				t.Fatalf("ErrInconsistent beklenirdi, gelen %v", err)
			}
		})
	}

	valid := WorkOrder{StartedQty: 5, GoodQty: 3, ScrapQty: 1, ReworkPending: 1}
	if err := valid.CheckInvariants(); err != nil {
		t.Fatalf("tutarlı iş emri kabul edilmeli: %v", err)
	}
}
