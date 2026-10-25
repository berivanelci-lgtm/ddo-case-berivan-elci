package workorder

import "context"

// Service, iş emri işlemlerini yürütür. Her işlem tek bir veritabanı işleminde çalışır:
// ya hepsi (durum, malzeme düşümü, kayıt) olur ya da hiçbiri.
type Service struct {
	store Store
}

// NewService, yeni bir iş emri servisi oluşturur.
func NewService(store Store) *Service {
	return &Service{store: store}
}

// Release, iş emrini üretime serbest bırakır. Ürünün geçerli bir reçetesi olmalıdır.
func (s *Service) Release(ctx context.Context, id int64) (WorkOrder, error) {
	return s.update(ctx, id, func(tx Tx, w WorkOrder) (WorkOrder, error) {
		released, err := w.Release()
		if err != nil {
			return WorkOrder{}, err
		}

		bom, err := tx.BOM(ctx)
		if err != nil {
			return WorkOrder{}, err
		}
		if _, err := Requirements(bom, 1); err != nil {
			return WorkOrder{}, err
		}
		return released, nil
	})
}

// Cancel, üretimi başlamamış iş emrini iptal eder.
func (s *Service) Cancel(ctx context.Context, id int64) (WorkOrder, error) {
	return s.update(ctx, id, func(_ Tx, w WorkOrder) (WorkOrder, error) {
		return w.Cancel()
	})
}

// RecordProduction, bir üretim kaydı işler ve başlayan her ürün için reçeteye göre malzeme düşer.
// Stok yetmezse hiçbir şey değişmez.
func (s *Service) RecordProduction(ctx context.Context, id, good, scrap, rework, userID int64) (WorkOrder, error) {
	return s.update(ctx, id, func(tx Tx, w WorkOrder) (WorkOrder, error) {
		updated, err := w.RecordProduction(good, scrap, rework)
		if err != nil {
			return WorkOrder{}, err
		}

		bom, err := tx.BOM(ctx)
		if err != nil {
			return WorkOrder{}, err
		}
		requirements, err := Requirements(bom, good+scrap+rework)
		if err != nil {
			return WorkOrder{}, err
		}
		if err := tx.Consume(ctx, requirements); err != nil {
			return WorkOrder{}, err
		}

		record := Record{Kind: RecordProduction, Good: good, Scrap: scrap, Rework: rework, UserID: userID}
		if err := tx.AddRecord(ctx, record); err != nil {
			return WorkOrder{}, err
		}
		return updated, nil
	})
}

// RecordRework, yeniden işlemde bekleyen ürünlerin sonucunu kaydeder.
// Yeniden işlem, ürün zaten başlamış olduğundan ek malzeme tüketmez.
func (s *Service) RecordRework(ctx context.Context, id, good, scrap, userID int64) (WorkOrder, error) {
	return s.update(ctx, id, func(tx Tx, w WorkOrder) (WorkOrder, error) {
		updated, err := w.RecordRework(good, scrap)
		if err != nil {
			return WorkOrder{}, err
		}

		record := Record{Kind: RecordRework, Good: good, Scrap: scrap, UserID: userID}
		if err := tx.AddRecord(ctx, record); err != nil {
			return WorkOrder{}, err
		}
		return updated, nil
	})
}

// Close, iş emrini kapatır. Kapanıştan önce tüketilen malzemenin
// başlayan ürün adedi × reçete miktarı ile tuttuğu denetlenir.
func (s *Service) Close(ctx context.Context, id int64, allowPartial bool) (WorkOrder, error) {
	return s.update(ctx, id, func(tx Tx, w WorkOrder) (WorkOrder, error) {
		closed, err := w.Close(allowPartial)
		if err != nil {
			return WorkOrder{}, err
		}

		bom, err := tx.BOM(ctx)
		if err != nil {
			return WorkOrder{}, err
		}
		consumed, err := tx.Consumed(ctx)
		if err != nil {
			return WorkOrder{}, err
		}
		if err := Reconcile(closed, bom, consumed); err != nil {
			return WorkOrder{}, err
		}
		return closed, nil
	})
}

// update, iş emrini kilitler, değişikliği uygular, sayaçların tuttuğunu denetler ve kaydeder.
func (s *Service) update(
	ctx context.Context,
	id int64,
	change func(tx Tx, current WorkOrder) (WorkOrder, error),
) (WorkOrder, error) {
	var result WorkOrder
	err := s.store.WithOrderLock(ctx, id, func(tx Tx) error {
		updated, err := change(tx, tx.Order())
		if err != nil {
			return err
		}
		if err := updated.CheckInvariants(); err != nil {
			return err
		}
		if err := tx.Save(ctx, updated); err != nil {
			return err
		}
		result = updated
		return nil
	})
	if err != nil {
		return WorkOrder{}, err
	}
	return result, nil
}
