package spc

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"testing"
)

var testCtx = context.Background()

// --- Bellekte çalışan sahte depo ---

type fakeState struct {
	points []Point
	limits []LimitSet
	alarms []Alarm
}

func (s fakeState) clone() fakeState {
	return fakeState{
		points: append([]Point(nil), s.points...),
		limits: append([]LimitSet(nil), s.limits...),
		alarms: append([]Alarm(nil), s.alarms...),
	}
}

type fakeStore struct {
	mu          sync.Mutex
	chart       Chart
	state       fakeState
	addPointErr error
}

var (
	_ Store   = (*fakeStore)(nil)
	_ ChartTx = (*fakeTx)(nil)
)

func newFakeStore(chart Chart) *fakeStore {
	return &fakeStore{chart: chart}
}

// WithChartLock, gerçek bir işlem gibi davranır: hata olursa değişiklikler geri alınır.
func (f *fakeStore) WithChartLock(_ context.Context, chartID int64, fn func(ChartTx) error) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if chartID != f.chart.ID {
		return ErrChartNotFound
	}
	snapshot := f.state.clone()
	if err := fn(&fakeTx{store: f}); err != nil {
		f.state = snapshot
		return err
	}
	return nil
}

type fakeTx struct{ store *fakeStore }

func (tx *fakeTx) Chart() Chart { return tx.store.chart }

func (tx *fakeTx) PointCount(context.Context) (int64, error) {
	return int64(len(tx.store.state.points)), nil
}

func (tx *fakeTx) FirstPoints(_ context.Context, n int) ([]Point, error) {
	points := tx.store.state.points
	if len(points) > n {
		points = points[:n]
	}
	return append([]Point(nil), points...), nil
}

func (tx *fakeTx) RecentPoints(_ context.Context, n int) ([]Point, error) {
	points := tx.store.state.points
	if len(points) > n {
		points = points[len(points)-n:]
	}
	return append([]Point(nil), points...), nil
}

func (tx *fakeTx) CurrentLimits(context.Context) (*LimitSet, error) {
	limits := tx.store.state.limits
	if len(limits) == 0 {
		return nil, nil
	}
	latest := limits[len(limits)-1]
	return &latest, nil
}

func (tx *fakeTx) AddPoint(_ context.Context, p Point) error {
	if tx.store.addPointErr != nil {
		return tx.store.addPointErr
	}
	tx.store.state.points = append(tx.store.state.points, p)
	return nil
}

func (tx *fakeTx) SaveLimits(_ context.Context, set LimitSet) error {
	tx.store.state.limits = append(tx.store.state.limits, set)
	return nil
}

func (tx *fakeTx) AddAlarms(_ context.Context, alarms []Alarm) error {
	tx.store.state.alarms = append(tx.store.state.alarms, alarms...)
	return nil
}

// --- Yardımcılar ---

func individualChart() Chart {
	return Chart{ID: 1, Name: "çap", Type: ChartIndividual, SubgroupSize: 1}
}

func newIndividualSetup() (*fakeStore, *Service) {
	store := newFakeStore(individualChart())
	return store, NewService(store)
}

func addValue(t *testing.T, svc *Service, values ...float64) Result {
	t.Helper()
	result, err := svc.AddMeasurement(testCtx, 1, values)
	if err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}
	return result
}

// baselineValues, dönüşümlü 9 ve 11 değerlerinden oluşan 25 ölçüm döner.
// Ortalama 9,96; ardışık farkların hepsi 2.
func baselineValues() []float64 {
	values := make([]float64, BaselineSize)
	for i := range values {
		if i%2 == 0 {
			values[i] = 9
		} else {
			values[i] = 11
		}
	}
	return values
}

func loadBaseline(t *testing.T, svc *Service) {
	t.Helper()
	for _, v := range baselineValues() {
		addValue(t, svc, v)
	}
}

// pseudoSeries, her çalıştırmada aynı olan sahte bir ölçüm serisi üretir.
// İçinde bir kayma, bir seviye değişimi ve bir aykırı değer vardır.
func pseudoSeries(n int) []float64 {
	state := uint32(12345)
	values := make([]float64, n)
	for i := range values {
		state = state*1664525 + 1013904223
		noise := float64(state>>8) / float64(1<<24)
		values[i] = 10 + (noise-0.5)*3

		if i >= 60 && i < 75 {
			values[i] += 0.04 * float64(i-60)
		}
		if i >= 150 && i < 170 {
			values[i] += 1.2
		}
		if i == 120 {
			values[i] += 6
		}
	}
	return values
}

// --- Testler ---

func TestAddMeasurement_NoLimitsBeforeBaselineIsComplete(t *testing.T) {
	store, svc := newIndividualSetup()

	for i, v := range baselineValues()[:BaselineSize-1] {
		result := addValue(t, svc, v)
		if result.Seq != int64(i+1) {
			t.Errorf("sıra %d olmalı, gelen %d", i+1, result.Seq)
		}
		if result.LimitsComputed || len(result.Alarms) != 0 {
			t.Fatalf("%d. ölçümde limit ya da alarm olmamalı", i+1)
		}
	}
	if len(store.state.limits) != 0 {
		t.Fatalf("25. noktadan önce limit kaydedilmemeli, kayıtlı %d", len(store.state.limits))
	}
}

func TestAddMeasurement_ComputesLimitsOnTwentyFifthPoint(t *testing.T) {
	store, svc := newIndividualSetup()

	var last Result
	for _, v := range baselineValues() {
		last = addValue(t, svc, v)
	}

	if !last.LimitsComputed || last.Limits == nil {
		t.Fatal("25. ölçümde limitler hesaplanmalı")
	}
	if last.Limits.Version != 1 || last.LimitVersion != 1 {
		t.Errorf("sürüm 1 olmalı, gelen %d", last.Limits.Version)
	}
	assertClose(t, "merkez", last.Limits.Mean.Center, 9.96)
	assertClose(t, "sigma", last.Limits.Mean.Sigma, 2/1.128)
	assertClose(t, "UCL", last.Limits.Mean.UCL, 9.96+3*2/1.128)
	if len(store.state.limits) != 1 {
		t.Errorf("1 limit sürümü kayıtlı olmalı, kayıtlı %d", len(store.state.limits))
	}
	if len(last.Alarms) != 0 {
		t.Error("taban çizgisinde alarm üretilmemeli")
	}
}

func TestAddMeasurement_RaisesAlarmWhenPointIsBeyondLimits(t *testing.T) {
	store, svc := newIndividualSetup()
	loadBaseline(t, svc)

	result := addValue(t, svc, 16)

	if len(result.Alarms) != 1 {
		t.Fatalf("1 alarm beklenirdi, gelen %d", len(result.Alarms))
	}
	alarm := result.Alarms[0]
	if alarm.Rule != RuleBeyond3Sigma || alarm.PointSeq != 26 || alarm.LimitVersion != 1 ||
		alarm.ChartID != 1 || alarm.Value != 16 {
		t.Errorf("beklenmeyen alarm: %+v", alarm)
	}
	if len(store.state.alarms) != 1 {
		t.Errorf("alarm kaydedilmeli, kayıtlı %d", len(store.state.alarms))
	}
}

// Her ölçüm ayrı bir çağrıyla gelir; dedektörün durumu her seferinde yeniden kurulur.
func TestAddMeasurement_NoDuplicateAlarmAcrossSeparateCalls(t *testing.T) {
	store, svc := newIndividualSetup()
	loadBaseline(t, svc)

	for i := 0; i < 12; i++ {
		addValue(t, svc, 10.5)
	}

	if len(store.state.alarms) != 1 {
		t.Fatalf("12 ardışık noktada tek alarm beklenirdi, gelen %d: %+v", len(store.state.alarms), store.state.alarms)
	}
	alarm := store.state.alarms[0]
	if alarm.Rule != RuleSameSide8 || alarm.PointSeq != 33 {
		t.Errorf("same_side_8 @ 33 beklenirdi, gelen %s @ %d", alarm.Rule, alarm.PointSeq)
	}
}

// Durumu son 8 noktadan geri kuran servis, kesintisiz çalışan bir dedektörle aynı alarmları üretmeli.
func TestAddMeasurement_MatchesContinuousDetection(t *testing.T) {
	values := pseudoSeries(250)
	store, svc := newIndividualSetup()
	for _, v := range values {
		addValue(t, svc, v)
	}

	limits, err := ComputeIndividual(values[:BaselineSize])
	if err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}
	detector, err := NewDetector(limits)
	if err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}

	var want []string
	for i, v := range values {
		for _, violation := range detector.Add(v) {
			if i >= BaselineSize {
				want = append(want, fmt.Sprintf("%s@%d", violation.Rule, i+1))
			}
		}
	}
	var got []string
	for _, alarm := range store.state.alarms {
		got = append(got, fmt.Sprintf("%s@%d", alarm.Rule, alarm.PointSeq))
	}

	if len(want) == 0 {
		t.Fatal("test serisi hiç alarm üretmedi, karşılaştırma anlamsız")
	}
	if len(got) != len(want) {
		t.Fatalf("%d alarm beklenirdi, gelen %d\nbeklenen: %v\ngelen: %v", len(want), len(got), want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%d. alarm: beklenen %s, gelen %s", i+1, want[i], got[i])
		}
	}
}

func TestAddMeasurement_XbarRComputesLimitsAndUsesSubgroupMean(t *testing.T) {
	store := newFakeStore(Chart{ID: 1, Name: "uzunluk", Type: ChartXbarR, SubgroupSize: 3})
	svc := NewService(store)

	var first, last Result
	for i := 0; i < BaselineSize; i++ {
		var result Result
		if i%2 == 0 {
			result = addValue(t, svc, 10, 12, 11)
		} else {
			result = addValue(t, svc, 9, 11, 10)
		}
		if i == 0 {
			first = result
		}
		last = result
	}

	assertClose(t, "ilk grup ortalaması", first.Statistic, 11)
	if !last.LimitsComputed || last.Limits == nil || last.Limits.Range == nil {
		t.Fatal("25. alt grupta hem X̄ hem R limitleri hesaplanmalı")
	}
	assertClose(t, "X̄ merkez", last.Limits.Mean.Center, 10.52)
	assertClose(t, "X̄ UCL", last.Limits.Mean.UCL, 12.566)
	assertClose(t, "R merkez", last.Limits.Range.Center, 2)
	if len(store.state.points[0].Subgroup) != 3 {
		t.Error("alt grubun ham değerleri saklanmalı")
	}
}

func TestAddMeasurement_RejectsWrongNumberOfValues(t *testing.T) {
	cases := map[string]struct {
		chart  Chart
		values []float64
	}{
		"bireysel grafikte iki değer": {individualChart(), []float64{1, 2}},
		"bireysel grafikte boş":       {individualChart(), nil},
		"X̄-R grafikte eksik grup":    {Chart{ID: 1, Name: "x", Type: ChartXbarR, SubgroupSize: 3}, []float64{1, 2}},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			store := newFakeStore(tc.chart)
			svc := NewService(store)

			_, err := svc.AddMeasurement(testCtx, 1, tc.values)

			if !errors.Is(err, ErrInvalidMeasurement) {
				t.Fatalf("ErrInvalidMeasurement beklenirdi, gelen %v", err)
			}
			if len(store.state.points) != 0 {
				t.Error("geçersiz ölçüm kaydedilmemeli")
			}
		})
	}
}

func TestAddMeasurement_RejectsNonFiniteValues(t *testing.T) {
	cases := map[string]float64{"NaN": math.NaN(), "+Inf": math.Inf(1), "-Inf": math.Inf(-1)}

	for name, value := range cases {
		t.Run(name, func(t *testing.T) {
			_, svc := newIndividualSetup()

			_, err := svc.AddMeasurement(testCtx, 1, []float64{value})

			if !errors.Is(err, ErrInvalidMeasurement) {
				t.Fatalf("ErrInvalidMeasurement beklenirdi, gelen %v", err)
			}
		})
	}
}

func TestAddMeasurement_ZeroVarianceBaselineIsRejectedAndRolledBack(t *testing.T) {
	store, svc := newIndividualSetup()
	for i := 0; i < BaselineSize-1; i++ {
		addValue(t, svc, 10)
	}

	_, err := svc.AddMeasurement(testCtx, 1, []float64{10})

	if !errors.Is(err, ErrZeroVariance) {
		t.Fatalf("ErrZeroVariance beklenirdi, gelen %v", err)
	}
	if len(store.state.points) != BaselineSize-1 {
		t.Fatalf("reddedilen ölçüm kaydedilmemeli, kayıtlı nokta %d", len(store.state.points))
	}

	result := addValue(t, svc, 10.5)
	if !result.LimitsComputed || result.Seq != BaselineSize {
		t.Errorf("değişimli 25. ölçümle limitler hesaplanmalı: %+v", result)
	}
}

func TestRecomputeLimits_CreatesNewVersionUsedByLaterAlarms(t *testing.T) {
	store, svc := newIndividualSetup()
	loadBaseline(t, svc)
	for _, v := range []float64{11, 9, 11, 9, 11} {
		addValue(t, svc, v)
	}
	if len(store.state.alarms) != 0 {
		t.Fatalf("ön koşul: alarm olmamalı, gelen %d", len(store.state.alarms))
	}

	set, err := svc.RecomputeLimits(testCtx, 1)
	if err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}

	if set.Version != 2 {
		t.Errorf("sürüm 2 olmalı, gelen %d", set.Version)
	}
	assertClose(t, "yeni merkez", set.Mean.Center, 10.04)
	if len(store.state.limits) != 2 {
		t.Fatalf("eski sürüm silinmemeli, kayıtlı %d", len(store.state.limits))
	}
	assertClose(t, "eski sürümün merkezi", store.state.limits[0].Mean.Center, 9.96)

	result := addValue(t, svc, 16)
	if len(result.Alarms) != 1 || result.Alarms[0].LimitVersion != 2 || result.LimitVersion != 2 {
		t.Errorf("alarm yeni limit sürümüne (2) bağlanmalı: %+v", result)
	}
}

func TestRecomputeLimits_RequiresBaseline(t *testing.T) {
	_, svc := newIndividualSetup()
	for i := 0; i < 10; i++ {
		addValue(t, svc, float64(i))
	}

	if _, err := svc.RecomputeLimits(testCtx, 1); !errors.Is(err, ErrNotEnoughData) {
		t.Fatalf("ErrNotEnoughData beklenirdi, gelen %v", err)
	}
}

func TestService_UnknownChart(t *testing.T) {
	_, svc := newIndividualSetup()

	if _, err := svc.AddMeasurement(testCtx, 99, []float64{1}); !errors.Is(err, ErrChartNotFound) {
		t.Errorf("AddMeasurement için ErrChartNotFound beklenirdi, gelen %v", err)
	}
	if _, err := svc.RecomputeLimits(testCtx, 99); !errors.Is(err, ErrChartNotFound) {
		t.Errorf("RecomputeLimits için ErrChartNotFound beklenirdi, gelen %v", err)
	}
}

func TestAddMeasurement_PropagatesStoreError(t *testing.T) {
	store, svc := newIndividualSetup()
	boom := errors.New("disk dolu")
	store.addPointErr = boom

	_, err := svc.AddMeasurement(testCtx, 1, []float64{10})

	if !errors.Is(err, boom) {
		t.Fatalf("depo hatası yukarı iletilmeli, gelen %v", err)
	}
}
