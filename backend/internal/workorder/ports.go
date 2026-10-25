package workorder

import (
	"context"
	"errors"
)

var (
	// ErrOrderNotFound, iş emri bulunamadığında döner.
	ErrOrderNotFound = errors.New("iş emri bulunamadı")
	// ErrProductNotFound, ürün bulunamadığında döner.
	ErrProductNotFound = errors.New("ürün bulunamadı")
	// ErrInsufficientStock, bir malzemenin stoğu yetmediğinde döner.
	ErrInsufficientStock = errors.New("stok yetersiz")
)

// RecordKind, bir kaydın üretim mi yoksa yeniden işlem sonucu mu olduğunu belirtir.
type RecordKind string

const (
	RecordProduction RecordKind = "production"
	RecordRework     RecordKind = "rework"
)

// Record, bir üretim ya da yeniden işlem kaydıdır.
type Record struct {
	Kind   RecordKind
	Good   int64
	Scrap  int64
	Rework int64
	UserID int64
}

// Tx, kilitlenmiş bir iş emri üzerindeki işlemlerdir.
// Hepsi aynı veritabanı işlemi (transaction) içinde çalışır.
type Tx interface {
	Order() WorkOrder
	BOM(ctx context.Context) ([]BOMLine, error)
	Save(ctx context.Context, w WorkOrder) error
	// Consume malzemeleri stoktan düşer ve iş emrinin tüketim kaydına ekler.
	// Herhangi bir malzemenin stoğu yetmezse ErrInsufficientStock döner.
	Consume(ctx context.Context, requirements []Requirement) error
	AddRecord(ctx context.Context, record Record) error
	// Consumed, iş emri için şimdiye kadar tüketilen malzemeleri döner.
	Consumed(ctx context.Context) (map[int64]int64, error)
}

// Store, bir iş emrini kilitleyip işlem içinde çalışmayı sağlar.
// fn hata dönerse yapılan tüm değişiklikler geri alınmalıdır.
type Store interface {
	WithOrderLock(ctx context.Context, orderID int64, fn func(tx Tx) error) error
}

// Repository, iş emirlerini oluşturma ve okuma işlemleridir.
type Repository interface {
	Create(ctx context.Context, productID, plannedQty, createdBy int64) (WorkOrder, error)
	Get(ctx context.Context, id int64) (WorkOrder, error)
	// List, kimliği before'dan küçük iş emirlerini yeniden eskiye sıralı döner.
	// before sıfırsa en yeni kayıtlardan başlar (keyset sayfalama).
	List(ctx context.Context, before int64, limit int) ([]WorkOrder, error)
}
