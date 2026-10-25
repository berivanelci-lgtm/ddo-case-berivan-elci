package workorder

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
)

var testCtx = context.Background()

// --- Bellekte çalışan sahte depo ---

type fakeState struct {
	order    WorkOrder
	stock    map[int64]int64
	consumed map[int64]int64
	records  []Record
}

func (s fakeState) clone() fakeState {
	stock := make(map[int64]int64, len(s.stock))
	for id, quantity := range s.stock {
		stock[id] = quantity
	}
	consumed := make(map[int64]int64, len(s.consumed))
	for id, quantity := range s.consumed {
		consumed[id] = quantity
	}
	return fakeState{
		order:    s.order,
		stock:    stock,
		consumed: consumed,
		records:  append([]Record(nil), s.records...),
	}
}

type fakeStore struct {
	mu    sync.Mutex
	bom   []BOMLine
	state fakeState
}

var (
	_ Store = (*fakeStore)(nil)
	_ Tx    = (*fakeTx)(nil)
)

// WithOrderLock, gerçek bir işlem gibi davranır: hata olursa değişiklikler geri alınır.
func (f *fakeStore) WithOrderLock(_ context.Context, orderID int64, fn func(Tx) error) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if orderID != f.state.order.ID {
		return ErrOrderNotFound
	}
	snapshot := f.state.clone()
	if err := fn(&fakeTx{store: f}); err != nil {
		f.state = snapshot
		return err
	}
	return nil
}

type fakeTx struct{ store *fakeStore }

func (tx *fakeTx) Order() WorkOrder { return tx.store.state.order }

func (tx *fakeTx) BOM(context.Context) ([]BOMLine, error) {
	return append([]BOMLine(nil), tx.store.bom...), nil
}

func (tx *fakeTx) Save(_ context.Context, w WorkOrder) error {
	tx.store.state.order = w
	return nil
}

func (tx *fakeTx) Consume(_ context.Context, requirements []Requirement) error {
	state := &tx.store.state
	for _, requirement := range requirements {
		if state.stock[requirement.MaterialID] < requirement.Quantity {
			return fmt.Errorf("%w: malzeme %d", ErrInsufficientStock, requirement.MaterialID)
		}
		state.stock[requirement.MaterialID] -= requirement.Quantity
		state.consumed[requirement.MaterialID] += requirement.Quantity
	}
	return nil
}

func (tx *fakeTx) AddRecord(_ context.Context, record Record) error {
	tx.store.state.records = append(tx.store.state.records, record)
	return nil
}

func (tx *fakeTx) Consumed(context.Context) (map[int64]int64, error) {
	copied := make(map[int64]int64, len(tx.store.state.consumed))
	for id, quantity := range tx.store.state.consumed {
		copied[id] = quantity
	}
	return copied, nil
}

// --- Yardımcılar ---

// newFixture, kimliği 1 olan planlanmış bir iş emri ve verilen stokla bir servis kurar.
func newFixture(t *testing.T, planned int64, stock map[int64]int64) (*fakeStore, *Service) {
	t.Helper()

	order := newOrder(t, planned)
	order.ID = 1

	copiedStock := make(map[int64]int64, len(stock))
	for id, quantity := range stock {
		copiedStock[id] = quantity
	}

	store := &fakeStore{
		bom:   testBOM(),
		state: fakeState{order: order, stock: copiedStock, consumed: map[int64]int64{}},
	}
	return store, NewService(store)
}

func mustServiceRelease(t *testing.T, svc *Service) WorkOrder {
	t.Helper()
	released, err := svc.Release(testCtx, 1)
	if err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}
	return released
}

func abundantStock() map[int64]int64 {
	return map[int64]int64{1: 1000, 2: 1000}
}

// --- Testler ---

func TestService_Release_RequiresBOM(t *testing.T) {
	store, svc := newFixture(t, 10, abundantStock())
	store.bom = nil

	_, err := svc.Release(testCtx, 1)

	if !errors.Is(err, ErrEmptyBOM) {
		t.Fatalf("ErrEmptyBOM beklenirdi, gelen %v", err)
	}
	if store.state.order.Status != StatusPlanned {
		t.Errorf("iş emri planlanmış kalmalı, gelen %s", store.state.order.Status)
	}
}

func TestService_UnknownOrder(t *testing.T) {
	_, svc := newFixture(t, 10, abundantStock())

	if _, err := svc.Release(testCtx, 99); !errors.Is(err, ErrOrderNotFound) {
		t.Errorf("Release: ErrOrderNotFound beklenirdi, gelen %v", err)
	}
	if _, err := svc.RecordProduction(testCtx, 99, 1, 0, 0, 5); !errors.Is(err, ErrOrderNotFound) {
		t.Errorf("RecordProduction: ErrOrderNotFound beklenirdi, gelen %v", err)
	}
	if _, err := svc.Close(testCtx, 99, false); !errors.Is(err, ErrOrderNotFound) {
		t.Errorf("Close: ErrOrderNotFound beklenirdi, gelen %v", err)
	}
}

func TestService_RecordProduction_ConsumesMaterialsPerBOM(t *testing.T) {
	store, svc := newFixture(t, 20, abundantStock())
	released := mustServiceRelease(t, svc)
	if released.Status != StatusReleased {
		t.Fatalf("released beklenirdi, gelen %s", released.Status)
	}

	// 10 ürün başlar: malzeme 1 için 10×5=50, malzeme 2 için 10×3=30.
	got, err := svc.RecordProduction(testCtx, 1, 8, 2, 0, 5)
	if err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}

	if got.Status != StatusInProgress || got.StartedQty != 10 || got.GoodQty != 8 || got.ScrapQty != 2 {
		t.Errorf("beklenmeyen iş emri: %+v", got)
	}
	state := store.state
	if state.stock[1] != 950 || state.stock[2] != 970 {
		t.Errorf("stok 950/970 olmalı, gelen %d/%d", state.stock[1], state.stock[2])
	}
	if state.consumed[1] != 50 || state.consumed[2] != 30 {
		t.Errorf("tüketim 50/30 olmalı, gelen %d/%d", state.consumed[1], state.consumed[2])
	}
	if len(state.records) != 1 || state.records[0].Kind != RecordProduction ||
		state.records[0].UserID != 5 || state.records[0].Good != 8 || state.records[0].Scrap != 2 {
		t.Errorf("beklenmeyen kayıtlar: %+v", state.records)
	}
}

func TestService_RecordProduction_InsufficientStockChangesNothing(t *testing.T) {
	// Malzeme 2 için 30 birim gerekiyor ama yalnızca 20 var.
	store, svc := newFixture(t, 20, map[int64]int64{1: 1000, 2: 20})
	mustServiceRelease(t, svc)

	_, err := svc.RecordProduction(testCtx, 1, 8, 2, 0, 5)

	if !errors.Is(err, ErrInsufficientStock) {
		t.Fatalf("ErrInsufficientStock beklenirdi, gelen %v", err)
	}
	state := store.state
	if state.stock[1] != 1000 || state.stock[2] != 20 {
		t.Errorf("stok değişmemeli, gelen %d/%d", state.stock[1], state.stock[2])
	}
	if len(state.consumed) != 0 || len(state.records) != 0 {
		t.Errorf("tüketim ve kayıt olmamalı: %+v %+v", state.consumed, state.records)
	}
	if state.order.Status != StatusReleased || state.order.StartedQty != 0 {
		t.Errorf("iş emri değişmemeli: %+v", state.order)
	}
}

func TestService_RecordProduction_RejectedOnPlannedOrder(t *testing.T) {
	store, svc := newFixture(t, 10, abundantStock())

	_, err := svc.RecordProduction(testCtx, 1, 1, 0, 0, 5)

	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("ErrInvalidTransition beklenirdi, gelen %v", err)
	}
	if store.state.stock[1] != 1000 || store.state.stock[2] != 1000 {
		t.Error("serbest bırakılmamış iş emri stok düşmemeli")
	}
}

func TestService_RecordProduction_InvalidQuantitiesChangeNothing(t *testing.T) {
	cases := map[string][3]int64{
		"negatif sağlam": {-1, 0, 0},
		"hepsi sıfır":    {0, 0, 0},
	}

	for name, quantities := range cases {
		t.Run(name, func(t *testing.T) {
			store, svc := newFixture(t, 10, abundantStock())
			mustServiceRelease(t, svc)

			_, err := svc.RecordProduction(testCtx, 1, quantities[0], quantities[1], quantities[2], 5)

			if !errors.Is(err, ErrInvalidQuantity) {
				t.Fatalf("ErrInvalidQuantity beklenirdi, gelen %v", err)
			}
			if store.state.stock[1] != 1000 || len(store.state.records) != 0 {
				t.Error("geçersiz miktar stok ya da kayıt değiştirmemeli")
			}
		})
	}
}

func TestService_RecordRework_DoesNotConsumeMaterials(t *testing.T) {
	store, svc := newFixture(t, 10, abundantStock())
	mustServiceRelease(t, svc)
	if _, err := svc.RecordProduction(testCtx, 1, 6, 0, 4, 5); err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}
	stockAfterProduction := store.state.stock[1]

	got, err := svc.RecordRework(testCtx, 1, 3, 1, 6)
	if err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}

	if got.GoodQty != 9 || got.ScrapQty != 1 || got.ReworkPending != 0 || got.StartedQty != 10 {
		t.Errorf("beklenmeyen sayaçlar: %+v", got)
	}
	if store.state.stock[1] != stockAfterProduction || store.state.consumed[1] != 50 {
		t.Error("yeniden işlem ek malzeme tüketmemeli")
	}
	if len(store.state.records) != 2 || store.state.records[1].Kind != RecordRework || store.state.records[1].UserID != 6 {
		t.Errorf("ikinci kayıt yeniden işlem olmalı: %+v", store.state.records)
	}
}

func TestService_Close_ReconcilesAndCompletes(t *testing.T) {
	store, svc := newFixture(t, 10, abundantStock())
	mustServiceRelease(t, svc)
	// 12 ürün başlar: 10 sağlam, 2 fire. Tüketim 60 ve 36 olmalı.
	if _, err := svc.RecordProduction(testCtx, 1, 10, 2, 0, 5); err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}

	closed, err := svc.Close(testCtx, 1, false)
	if err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}

	if closed.Status != StatusCompleted {
		t.Errorf("completed beklenirdi, gelen %s", closed.Status)
	}
	if store.state.consumed[1] != 60 || store.state.consumed[2] != 36 {
		t.Errorf("tüketim 60/36 olmalı, gelen %d/%d", store.state.consumed[1], store.state.consumed[2])
	}
	if err := Reconcile(store.state.order, store.bom, store.state.consumed); err != nil {
		t.Errorf("kapanıştan sonra miktarlar tutmalı: %v", err)
	}
}

func TestService_Close_DetectsTamperedConsumption(t *testing.T) {
	store, svc := newFixture(t, 10, abundantStock())
	mustServiceRelease(t, svc)
	if _, err := svc.RecordProduction(testCtx, 1, 10, 0, 0, 5); err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}
	store.state.consumed[1]-- // tüketim kaydı bozuldu

	_, err := svc.Close(testCtx, 1, false)

	if !errors.Is(err, ErrInconsistent) {
		t.Fatalf("ErrInconsistent beklenirdi, gelen %v", err)
	}
	if store.state.order.Status != StatusInProgress {
		t.Errorf("tutarsız iş emri kapanmamalı, durum %s", store.state.order.Status)
	}
}

func TestService_Close_PartialNeedsApproval(t *testing.T) {
	store, svc := newFixture(t, 10, abundantStock())
	mustServiceRelease(t, svc)
	if _, err := svc.RecordProduction(testCtx, 1, 6, 0, 0, 5); err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}

	if _, err := svc.Close(testCtx, 1, false); !errors.Is(err, ErrPartialNotAllowed) {
		t.Fatalf("ErrPartialNotAllowed beklenirdi, gelen %v", err)
	}
	if store.state.order.Status != StatusInProgress {
		t.Fatalf("onaysız kapanış iş emrini kapatmamalı, durum %s", store.state.order.Status)
	}

	closed, err := svc.Close(testCtx, 1, true)
	if err != nil {
		t.Fatalf("onaylı kısmi kapanış başarılı olmalı: %v", err)
	}
	if closed.Status != StatusCompleted || closed.GoodQty != 6 {
		t.Errorf("beklenmeyen kapanış: %+v", closed)
	}
}

func TestService_Cancel_OnlyBeforeProduction(t *testing.T) {
	t.Run("üretimden önce", func(t *testing.T) {
		store, svc := newFixture(t, 10, abundantStock())
		mustServiceRelease(t, svc)

		cancelled, err := svc.Cancel(testCtx, 1)
		if err != nil {
			t.Fatalf("beklenmeyen hata: %v", err)
		}
		if cancelled.Status != StatusCancelled {
			t.Errorf("cancelled beklenirdi, gelen %s", cancelled.Status)
		}
		if store.state.stock[1] != 1000 {
			t.Error("iptal stoğu değiştirmemeli")
		}
	})

	t.Run("üretimden sonra", func(t *testing.T) {
		store, svc := newFixture(t, 10, abundantStock())
		mustServiceRelease(t, svc)
		if _, err := svc.RecordProduction(testCtx, 1, 3, 0, 0, 5); err != nil {
			t.Fatalf("beklenmeyen hata: %v", err)
		}

		if _, err := svc.Cancel(testCtx, 1); !errors.Is(err, ErrInvalidTransition) {
			t.Fatalf("ErrInvalidTransition beklenirdi, gelen %v", err)
		}
		if store.state.stock[1] != 985 {
			t.Errorf("düşülen malzeme geri verilmemeli, stok %d", store.state.stock[1])
		}
	})
}

// Bu test servis mantığını sahte depoyla dener. Gerçek PostgreSQL ile yarış kanıtı
// HTTP uçları eklendikten sonra paralel istek betiğiyle yapılacaktır.
func TestService_ConcurrentProduction_NeverOversells(t *testing.T) {
	// Malzeme 1 stoğu 100, ürün başına 5 birim: en fazla 20 ürün başlayabilir.
	store, svc := newFixture(t, 100, map[int64]int64{1: 100, 2: 1000})
	mustServiceRelease(t, svc)

	const attempts = 30
	var succeeded, outOfStock atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := svc.RecordProduction(testCtx, 1, 1, 0, 0, 5)
			switch {
			case err == nil:
				succeeded.Add(1)
			case errors.Is(err, ErrInsufficientStock):
				outOfStock.Add(1)
			default:
				t.Errorf("beklenmeyen hata: %v", err)
			}
		}()
	}
	wg.Wait()

	state := store.state
	if succeeded.Load() != 20 || outOfStock.Load() != 10 {
		t.Errorf("20 başarılı ve 10 stok yetersiz beklenirdi, gelen %d ve %d", succeeded.Load(), outOfStock.Load())
	}
	if state.stock[1] != 0 || state.stock[2] != 940 {
		t.Errorf("stok 0/940 olmalı, gelen %d/%d", state.stock[1], state.stock[2])
	}
	if state.order.StartedQty != 20 || len(state.records) != 20 {
		t.Errorf("20 ürün ve 20 kayıt beklenirdi, gelen %d ve %d", state.order.StartedQty, len(state.records))
	}
	if err := Reconcile(state.order, store.bom, state.consumed); err != nil {
		t.Errorf("kapanıştan önce miktarlar tutmalı: %v", err)
	}
}
