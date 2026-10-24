package spc

import (
	"context"
	"errors"
	"fmt"
	"math"
)

var (
	// ErrChartNotFound, grafik bulunamadığında döner.
	ErrChartNotFound = errors.New("kontrol grafiği bulunamadı")
	// ErrInvalidMeasurement, ölçüm değerleri geçersiz olduğunda döner.
	ErrInvalidMeasurement = errors.New("ölçüm geçersiz")
	// ErrZeroVariance, ilk ölçümlerde hiç değişim olmadığı için limit hesaplanamadığında döner.
	ErrZeroVariance = errors.New("ilk ölçümlerde hiç değişim yok, limit hesaplanamaz")
)

// ChartTx, kilitlenmiş bir grafik üzerinde yapılan işlemlerdir.
// Hepsi aynı veritabanı işlemi (transaction) içinde çalışır.
type ChartTx interface {
	Chart() Chart
	PointCount(ctx context.Context) (int64, error)
	// FirstPoints ve RecentPoints noktaları eskiden yeniye sıralı döner.
	FirstPoints(ctx context.Context, n int) ([]Point, error)
	RecentPoints(ctx context.Context, n int) ([]Point, error)
	// CurrentLimits, en son limit sürümünü döner; hiç yoksa nil döner.
	CurrentLimits(ctx context.Context) (*LimitSet, error)
	AddPoint(ctx context.Context, p Point) error
	SaveLimits(ctx context.Context, set LimitSet) error
	AddAlarms(ctx context.Context, alarms []Alarm) error
}

// Store, bir grafiği kilitleyip işlem içinde çalışmayı sağlar.
// fn hata dönerse yapılan tüm değişiklikler geri alınmalıdır.
type Store interface {
	WithChartLock(ctx context.Context, chartID int64, fn func(tx ChartTx) error) error
}

// Result, bir ölçüm eklendikten sonraki durumu özetler.
type Result struct {
	Seq            int64
	Statistic      float64
	Alarms         []Alarm
	LimitsComputed bool // bu ölçümle ilk limitler hesaplandıysa true
	LimitVersion   int  // geçerli limit sürümü, henüz yoksa 0
	Limits         *LimitSet
}

// Service, ölçüm ekleme ve limit yenileme işlemlerini yürütür.
type Service struct {
	store Store
}

// NewService, yeni bir SPC servisi oluşturur.
func NewService(store Store) *Service {
	return &Service{store: store}
}

// AddMeasurement, grafiğe yeni bir ölçüm (ya da alt grup) ekler ve kuralları değerlendirir.
func (s *Service) AddMeasurement(ctx context.Context, chartID int64, subgroup []float64) (Result, error) {
	var result Result
	err := s.store.WithChartLock(ctx, chartID, func(tx ChartTx) error {
		var err error
		result, err = addMeasurement(ctx, tx, subgroup)
		return err
	})
	if err != nil {
		return Result{}, err
	}
	return result, nil
}

// RecomputeLimits, son 25 noktadan yeni bir limit sürümü üretir. Eski sürümler saklanır.
func (s *Service) RecomputeLimits(ctx context.Context, chartID int64) (LimitSet, error) {
	var set LimitSet
	err := s.store.WithChartLock(ctx, chartID, func(tx ChartTx) error {
		current, err := tx.CurrentLimits(ctx)
		if err != nil {
			return err
		}
		if current == nil {
			return fmt.Errorf("%w: ilk limitler henüz hesaplanmadı", ErrNotEnoughData)
		}

		points, err := tx.RecentPoints(ctx, BaselineSize)
		if err != nil {
			return err
		}
		if len(points) < BaselineSize {
			return fmt.Errorf("%w: en az %d nokta gerekli, mevcut %d", ErrNotEnoughData, BaselineSize, len(points))
		}

		set, err = computeLimitSet(tx.Chart(), points, current.Version+1)
		if err != nil {
			return err
		}
		return tx.SaveLimits(ctx, set)
	})
	if err != nil {
		return LimitSet{}, err
	}
	return set, nil
}

func addMeasurement(ctx context.Context, tx ChartTx, subgroup []float64) (Result, error) {
	chart := tx.Chart()
	if err := validateSubgroup(chart, subgroup); err != nil {
		return Result{}, err
	}

	count, err := tx.PointCount(ctx)
	if err != nil {
		return Result{}, err
	}
	// Dedektörün durumunu geri kurmak için, yeni nokta eklenmeden önceki son noktalar okunur.
	recent, err := tx.RecentPoints(ctx, maxWindow)
	if err != nil {
		return Result{}, err
	}
	limits, err := tx.CurrentLimits(ctx)
	if err != nil {
		return Result{}, err
	}

	point := Point{
		Seq:       count + 1,
		Subgroup:  append([]float64(nil), subgroup...),
		Statistic: mean(subgroup),
	}
	if err := tx.AddPoint(ctx, point); err != nil {
		return Result{}, err
	}
	result := Result{Seq: point.Seq, Statistic: point.Statistic}

	if limits == nil {
		if point.Seq < BaselineSize {
			return result, nil
		}
		return computeInitialLimits(ctx, tx, result)
	}

	result.LimitVersion = limits.Version

	detector, err := NewDetector(limits.Mean)
	if err != nil {
		return Result{}, err
	}
	for _, previous := range recent {
		detector.Add(previous.Statistic)
	}

	var alarms []Alarm
	for _, violation := range detector.Add(point.Statistic) {
		alarms = append(alarms, Alarm{
			ChartID:      chart.ID,
			PointSeq:     point.Seq,
			Rule:         violation.Rule,
			Value:        violation.Value,
			LimitVersion: limits.Version,
		})
	}
	if len(alarms) > 0 {
		if err := tx.AddAlarms(ctx, alarms); err != nil {
			return Result{}, err
		}
	}
	result.Alarms = alarms
	return result, nil
}

// computeInitialLimits, ilk 25 noktadan limitleri hesaplayıp sürüm 1 olarak kaydeder.
func computeInitialLimits(ctx context.Context, tx ChartTx, result Result) (Result, error) {
	points, err := tx.FirstPoints(ctx, BaselineSize)
	if err != nil {
		return Result{}, err
	}
	set, err := computeLimitSet(tx.Chart(), points, 1)
	if err != nil {
		return Result{}, err
	}
	if err := tx.SaveLimits(ctx, set); err != nil {
		return Result{}, err
	}

	result.LimitsComputed = true
	result.LimitVersion = set.Version
	result.Limits = &set
	return result, nil
}

func computeLimitSet(chart Chart, points []Point, version int) (LimitSet, error) {
	set := LimitSet{Version: version}

	switch chart.Type {
	case ChartIndividual:
		values := make([]float64, len(points))
		for i, p := range points {
			values[i] = p.Statistic
		}
		limits, err := ComputeIndividual(values)
		if err != nil {
			return LimitSet{}, err
		}
		set.Mean = limits

	case ChartXbarR:
		groups := make([][]float64, len(points))
		for i, p := range points {
			groups[i] = p.Subgroup
		}
		computed, err := ComputeXbarR(groups)
		if err != nil {
			return LimitSet{}, err
		}
		set.Mean = computed.Mean
		rangeLimits := computed.Range
		set.Range = &rangeLimits

	default:
		return LimitSet{}, fmt.Errorf("%w: bilinmeyen grafik türü %q", ErrInvalidMeasurement, chart.Type)
	}

	if set.Mean.Sigma <= 0 {
		return LimitSet{}, ErrZeroVariance
	}
	return set, nil
}

func validateSubgroup(chart Chart, subgroup []float64) error {
	want := 1
	if chart.Type == ChartXbarR {
		want = chart.SubgroupSize
	}
	if len(subgroup) != want {
		return fmt.Errorf("%w: %d değer beklenirdi, gelen %d", ErrInvalidMeasurement, want, len(subgroup))
	}
	for _, v := range subgroup {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return fmt.Errorf("%w: değerler sonlu birer sayı olmalı", ErrInvalidMeasurement)
		}
	}
	return nil
}
