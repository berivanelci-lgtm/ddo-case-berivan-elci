// Package workorder, iş emri durum makinesini ve malzeme hesaplarını içerir.
// Bu paket saf mantıktır: veritabanı ve HTTP bilmez, bu yüzden kolay test edilir.
package workorder

import (
	"errors"
	"fmt"
)

// MaxQuantity, tek bir miktar alanı için kabul edilen en büyük değerdir.
// Toplamların taşmaması için sınırlıdır.
const MaxQuantity int64 = 1_000_000_000

// Status, iş emrinin durumunu belirtir.
type Status string

const (
	StatusPlanned    Status = "planned"
	StatusReleased   Status = "released"
	StatusInProgress Status = "in_progress"
	StatusCompleted  Status = "completed"
	StatusCancelled  Status = "cancelled"
)

var (
	// ErrInvalidTransition, işlem iş emrinin mevcut durumunda yapılamıyorsa döner.
	ErrInvalidTransition = errors.New("bu durumda bu işlem yapılamaz")
	// ErrInvalidQuantity, miktar geçersiz olduğunda döner.
	ErrInvalidQuantity = errors.New("miktar geçersiz")
	// ErrExceedsPlan, işlem planlanan miktarı aşacaksa döner.
	ErrExceedsPlan = errors.New("planlanan miktar aşılamaz")
	// ErrReworkPending, yeniden işlem bekleyen ürünler varken kapatılmak istenirse döner.
	ErrReworkPending = errors.New("yeniden işlem bekleyen ürünler var")
	// ErrPartialNotAllowed, plan tamamlanmadan kapatılmak istenip onay verilmediyse döner.
	ErrPartialNotAllowed = errors.New("planlanan miktar tamamlanmadı, kısmi kapanış onaylanmadı")
	// ErrInconsistent, miktarlar birbiriyle tutmadığında döner.
	ErrInconsistent = errors.New("miktarlar tutarsız")
)

// WorkOrder, bir iş emrinin durumunu ve sayaçlarını tutar.
// Değişmez kural: StartedQty = GoodQty + ScrapQty + ReworkPending.
// Metotlar değer alıcıdır ve yeni durumu döner; mevcut değer değişmez.
type WorkOrder struct {
	ID            int64
	ProductID     int64
	PlannedQty    int64
	Status        Status
	StartedQty    int64 // üretime başlayan toplam ürün sayısı
	GoodQty       int64 // sağlam çıkan ürünler (yeniden işlemden gelenler dahil)
	ScrapQty      int64 // fire
	ReworkPending int64 // yeniden işlem sonucunu bekleyen ürünler
}

// New, planlanmış yeni bir iş emri oluşturur.
func New(productID, plannedQty int64) (WorkOrder, error) {
	if plannedQty <= 0 || plannedQty > MaxQuantity {
		return WorkOrder{}, fmt.Errorf("%w: planlanan miktar 1 ile %d arasında olmalı", ErrInvalidQuantity, MaxQuantity)
	}
	return WorkOrder{ProductID: productID, PlannedQty: plannedQty, Status: StatusPlanned}, nil
}

// Release, planlanmış iş emrini üretime serbest bırakır.
func (w WorkOrder) Release() (WorkOrder, error) {
	if w.Status != StatusPlanned {
		return WorkOrder{}, transitionError(w.Status, "serbest bırakma")
	}
	w.Status = StatusReleased
	return w, nil
}

// Cancel, henüz üretim başlamamış iş emrini iptal eder.
// Üretim başladıysa malzeme düşmüştür; bu durumda iş emri iptal edilmez, kısmi olarak kapatılır.
func (w WorkOrder) Cancel() (WorkOrder, error) {
	if w.Status != StatusPlanned && w.Status != StatusReleased {
		return WorkOrder{}, transitionError(w.Status, "iptal")
	}
	w.Status = StatusCancelled
	return w, nil
}

// RecordProduction, bir üretim kaydını işler. Üç adet de üretime başlamış ürünlerdir:
// sağlam çıkanlar, fire olanlar ve yeniden işleme gönderilenler.
// İlk kayıtla iş emri "in_progress" durumuna geçer.
func (w WorkOrder) RecordProduction(good, scrap, rework int64) (WorkOrder, error) {
	if w.Status != StatusReleased && w.Status != StatusInProgress {
		return WorkOrder{}, transitionError(w.Status, "üretim kaydı")
	}
	if err := checkQuantity("sağlam adet", good); err != nil {
		return WorkOrder{}, err
	}
	if err := checkQuantity("fire adedi", scrap); err != nil {
		return WorkOrder{}, err
	}
	if err := checkQuantity("yeniden işlem adedi", rework); err != nil {
		return WorkOrder{}, err
	}

	units := good + scrap + rework
	if units == 0 {
		return WorkOrder{}, fmt.Errorf("%w: en az bir ürün kaydedilmeli", ErrInvalidQuantity)
	}

	// Fire plana sayılmaz: fire olunca hedefe ulaşmak için fazladan ürün başlatmak gerekir.
	if w.GoodQty+w.ReworkPending+good+rework > w.PlannedQty {
		return WorkOrder{}, fmt.Errorf("%w: planlanan %d, sağlam ve bekleyen toplamı %d olurdu",
			ErrExceedsPlan, w.PlannedQty, w.GoodQty+w.ReworkPending+good+rework)
	}

	w.StartedQty += units
	w.GoodQty += good
	w.ScrapQty += scrap
	w.ReworkPending += rework
	w.Status = StatusInProgress
	return w, nil
}

// RecordRework, yeniden işlemde bekleyen ürünlerin sonucunu kaydeder.
// Her ürün ya sağlam olur ya da fire olur; sonuçlanmayanlar bekleyen olarak kalır.
func (w WorkOrder) RecordRework(good, scrap int64) (WorkOrder, error) {
	if w.Status != StatusInProgress {
		return WorkOrder{}, transitionError(w.Status, "yeniden işlem sonucu girme")
	}
	if err := checkQuantity("sağlam adet", good); err != nil {
		return WorkOrder{}, err
	}
	if err := checkQuantity("fire adedi", scrap); err != nil {
		return WorkOrder{}, err
	}

	resolved := good + scrap
	if resolved == 0 {
		return WorkOrder{}, fmt.Errorf("%w: en az bir ürünün sonucu girilmeli", ErrInvalidQuantity)
	}
	if resolved > w.ReworkPending {
		return WorkOrder{}, fmt.Errorf("%w: yeniden işlemde %d ürün bekliyor, %d sonuç girildi",
			ErrInvalidQuantity, w.ReworkPending, resolved)
	}

	w.GoodQty += good
	w.ScrapQty += scrap
	w.ReworkPending -= resolved
	return w, nil
}

// Close, iş emrini kapatır. Bekleyen yeniden işlem olmamalıdır.
// Sağlam adet plandan azsa kapatmak için allowPartial açıkça true verilmelidir (kısmi tamamlama).
func (w WorkOrder) Close(allowPartial bool) (WorkOrder, error) {
	if w.Status != StatusInProgress {
		return WorkOrder{}, transitionError(w.Status, "kapatma")
	}
	if err := w.CheckInvariants(); err != nil {
		return WorkOrder{}, err
	}
	if w.ReworkPending > 0 {
		return WorkOrder{}, fmt.Errorf("%w: %d ürün sonuç bekliyor", ErrReworkPending, w.ReworkPending)
	}
	if w.GoodQty < w.PlannedQty && !allowPartial {
		return WorkOrder{}, fmt.Errorf("%w: planlanan %d, tamamlanan %d",
			ErrPartialNotAllowed, w.PlannedQty, w.GoodQty)
	}
	w.Status = StatusCompleted
	return w, nil
}

// CheckInvariants, sayaçların birbiriyle tuttuğunu denetler.
func (w WorkOrder) CheckInvariants() error {
	if w.StartedQty < 0 || w.GoodQty < 0 || w.ScrapQty < 0 || w.ReworkPending < 0 {
		return fmt.Errorf("%w: negatif miktar", ErrInconsistent)
	}
	if sum := w.GoodQty + w.ScrapQty + w.ReworkPending; w.StartedQty != sum {
		return fmt.Errorf("%w: başlayan %d, sağlam+fire+bekleyen toplamı %d", ErrInconsistent, w.StartedQty, sum)
	}
	return nil
}

func transitionError(status Status, action string) error {
	return fmt.Errorf("%w: %q durumundaki iş emrinde %s yapılamaz", ErrInvalidTransition, status, action)
}

func checkQuantity(name string, value int64) error {
	if value < 0 || value > MaxQuantity {
		return fmt.Errorf("%w: %s 0 ile %d arasında olmalı", ErrInvalidQuantity, name, MaxQuantity)
	}
	return nil
}
