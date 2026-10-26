// Package quality, kalite ölçüm formlarını, tolerans değerlendirmesini ve ölçüm girişini yönetir.
package quality

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/spc"
)

const maxItems = 30

var (
	ErrInvalid            = errors.New("kalite formu ya da ölçüm geçersiz")
	ErrNotFound           = errors.New("kalite formu bulunamadı")
	ErrExists             = errors.New("bu kodla bir kalite formu zaten var")
	ErrCalibrationExpired = errors.New("ölçüm cihazının kalibrasyon süresi dolmuş")
)

// Item, formdaki bir karakteristiktir (örn. çap 10 ± 0,5 mm).
type Item struct {
	Position       int
	Characteristic string
	Unit           string
	Nominal        float64
	LSL            float64
	USL            float64
	ChartID        *int64 // bağlıysa değer bu SPC grafiğine ölçüm olarak gider
}

// Form, bir kalite ölçüm formudur.
type Form struct {
	ID             int64
	Code           string
	Name           string
	GaugeCode      string
	CalibrationDue *time.Time
	Items          []Item
}

// Value, bir muayenedeki tek karakteristiğin sonucudur.
type Value struct {
	Position       int
	Characteristic string
	Value          float64
	InSpec         bool
}

// Inspection, formun bir kez doldurulmasıdır.
type Inspection struct {
	ID          int64
	FormID      int64
	InspectedBy int64
	InspectedAt time.Time
	WorkOrderID *int64
	AllInSpec   bool
	Values      []Value
}

// ChartOutcome, değerin bağlı SPC grafiğine yazılmasının sonucudur.
type ChartOutcome struct {
	Position int
	ChartID  int64
	Seq      int64
	Alarms   []spc.Alarm
	Error    string
}

// Repository, form ve muayenelerin saklandığı yerdir.
type Repository interface {
	CreateForm(ctx context.Context, f Form, createdBy int64) (Form, error)
	ListForms(ctx context.Context) ([]Form, error)
	GetForm(ctx context.Context, id int64) (Form, error)
	SaveInspection(ctx context.Context, in Inspection) (Inspection, error)
	RecentInspections(ctx context.Context, formID int64, limit int) ([]Inspection, error)
}

// MeasurementRecorder, bir SPC grafiğine ölçüm ekleyen her şeydir.
type MeasurementRecorder interface {
	AddMeasurement(ctx context.Context, chartID int64, subgroup []float64) (spc.Result, error)
}

// ValidateForm, formu doğrular, metinleri temizler ve karakteristik sıralarını 1'den numaralar.
func ValidateForm(f Form) (Form, error) {
	f.Code = strings.TrimSpace(f.Code)
	f.Name = strings.TrimSpace(f.Name)
	f.GaugeCode = strings.TrimSpace(f.GaugeCode)
	switch {
	case f.Code == "" || utf8.RuneCountInString(f.Code) > 40:
		return Form{}, fmt.Errorf("%w: kod 1 ile 40 karakter arasında olmalı", ErrInvalid)
	case f.Name == "" || utf8.RuneCountInString(f.Name) > 100:
		return Form{}, fmt.Errorf("%w: ad 1 ile 100 karakter arasında olmalı", ErrInvalid)
	case utf8.RuneCountInString(f.GaugeCode) > 40:
		return Form{}, fmt.Errorf("%w: cihaz kodu en fazla 40 karakter olmalı", ErrInvalid)
	case len(f.Items) == 0 || len(f.Items) > maxItems:
		return Form{}, fmt.Errorf("%w: form 1 ile %d karakteristik içermeli", ErrInvalid, maxItems)
	}

	items := make([]Item, 0, len(f.Items))
	for i, item := range f.Items {
		item.Position = i + 1
		item.Characteristic = strings.TrimSpace(item.Characteristic)
		item.Unit = strings.TrimSpace(item.Unit)
		switch {
		case item.Characteristic == "" || utf8.RuneCountInString(item.Characteristic) > 100:
			return Form{}, fmt.Errorf("%w: %d. karakteristiğin adı 1 ile 100 karakter arasında olmalı", ErrInvalid, i+1)
		case utf8.RuneCountInString(item.Unit) > 20:
			return Form{}, fmt.Errorf("%w: %d. karakteristiğin birimi en fazla 20 karakter olmalı", ErrInvalid, i+1)
		case !finite(item.Nominal, item.LSL, item.USL):
			return Form{}, fmt.Errorf("%w: %d. karakteristiğin sınırları sonlu sayı olmalı", ErrInvalid, i+1)
		case item.LSL >= item.USL:
			return Form{}, fmt.Errorf("%w: %d. karakteristikte alt sınır üst sınırdan küçük olmalı", ErrInvalid, i+1)
		case item.Nominal < item.LSL || item.Nominal > item.USL:
			return Form{}, fmt.Errorf("%w: %d. karakteristikte nominal değer sınırların arasında olmalı", ErrInvalid, i+1)
		case item.ChartID != nil && *item.ChartID <= 0:
			return Form{}, fmt.Errorf("%w: %d. karakteristiğin grafik kimliği pozitif olmalı", ErrInvalid, i+1)
		}
		items = append(items, item)
	}
	f.Items = items
	return f, nil
}

// Evaluate, kalibrasyonu ve her değerin tolerans içinde olup olmadığını denetler.
// Kalibrasyon bitiş günü dahil geçerlidir; ertesi gün ölçüm reddedilir.
func Evaluate(f Form, values []float64, today time.Time) ([]Value, bool, error) {
	if f.CalibrationDue != nil && dateOnly(today).After(dateOnly(*f.CalibrationDue)) {
		return nil, false, fmt.Errorf("%w: %s cihazının kalibrasyonu %s tarihinde bitti",
			ErrCalibrationExpired, f.GaugeCode, f.CalibrationDue.Format("2006-01-02"))
	}
	if len(values) != len(f.Items) {
		return nil, false, fmt.Errorf("%w: %d değer beklenirdi, gelen %d", ErrInvalid, len(f.Items), len(values))
	}

	results := make([]Value, 0, len(values))
	allInSpec := true
	for i, item := range f.Items {
		if !finite(values[i]) {
			return nil, false, fmt.Errorf("%w: %d. değer sonlu bir sayı olmalı", ErrInvalid, i+1)
		}
		inSpec := values[i] >= item.LSL && values[i] <= item.USL
		allInSpec = allInSpec && inSpec
		results = append(results, Value{
			Position: item.Position, Characteristic: item.Characteristic, Value: values[i], InSpec: inSpec,
		})
	}
	return results, allInSpec, nil
}

// Service, form ve muayene işlemlerini yürütür.
type Service struct {
	repo   Repository
	charts MeasurementRecorder
	now    func() time.Time
}

// NewService, yeni bir Service oluşturur.
func NewService(repo Repository, charts MeasurementRecorder) *Service {
	return &Service{repo: repo, charts: charts, now: time.Now}
}

// CreateForm, formu doğrulayıp kaydeder.
func (s *Service) CreateForm(ctx context.Context, f Form, createdBy int64) (Form, error) {
	cleaned, err := ValidateForm(f)
	if err != nil {
		return Form{}, err
	}
	return s.repo.CreateForm(ctx, cleaned, createdBy)
}

// ListForms, formları döner.
func (s *Service) ListForms(ctx context.Context) ([]Form, error) { return s.repo.ListForms(ctx) }

// GetForm, formu döner.
func (s *Service) GetForm(ctx context.Context, id int64) (Form, error) {
	return s.repo.GetForm(ctx, id)
}

// RecentInspections, formun son muayenelerini döner.
func (s *Service) RecentInspections(ctx context.Context, formID int64, limit int) ([]Inspection, error) {
	if _, err := s.repo.GetForm(ctx, formID); err != nil {
		return nil, err
	}
	return s.repo.RecentInspections(ctx, formID, limit)
}

// Inspect, formun bir kez doldurulmasını kaydeder ve grafiğe bağlı değerleri SPC'ye gönderir.
// Muayene ve SPC ölçümleri ayrı işlemlerde yazılır; SPC hatası muayeneyi geri almaz, sonuçta görünür.
func (s *Service) Inspect(ctx context.Context, formID int64, values []float64, workOrderID *int64, userID int64) (Inspection, []ChartOutcome, error) {
	if workOrderID != nil && *workOrderID <= 0 {
		return Inspection{}, nil, fmt.Errorf("%w: iş emri kimliği pozitif olmalı", ErrInvalid)
	}
	form, err := s.repo.GetForm(ctx, formID)
	if err != nil {
		return Inspection{}, nil, err
	}
	results, allInSpec, err := Evaluate(form, values, s.now())
	if err != nil {
		return Inspection{}, nil, err
	}

	saved, err := s.repo.SaveInspection(ctx, Inspection{
		FormID: formID, InspectedBy: userID, WorkOrderID: workOrderID, AllInSpec: allInSpec, Values: results,
	})
	if err != nil {
		return Inspection{}, nil, err
	}

	outcomes := make([]ChartOutcome, 0)
	for i, item := range form.Items {
		if item.ChartID == nil {
			continue
		}
		outcome := ChartOutcome{Position: item.Position, ChartID: *item.ChartID}
		result, err := s.charts.AddMeasurement(ctx, *item.ChartID, []float64{values[i]})
		if err != nil {
			outcome.Error = err.Error()
		} else {
			outcome.Seq = result.Seq
			outcome.Alarms = result.Alarms
		}
		outcomes = append(outcomes, outcome)
	}
	return saved, outcomes, nil
}

func finite(values ...float64) bool {
	for _, v := range values {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return false
		}
	}
	return true
}

func dateOnly(t time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
}
