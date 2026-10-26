package oee

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"
)

func at(hour, minute int) time.Time {
	return time.Date(2026, 10, 20, hour, minute, 0, 0, time.UTC)
}

func near(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 0.0001 {
		t.Errorf("%s: beklenen %.5f, gelen %.5f", name, want, got)
	}
}

func exampleCounts() []Count {
	return []Count{
		{At: at(8, 0), Good: 150, Scrap: 5},
		{At: at(12, 0), Good: 135, Scrap: 10},
		{At: at(18, 0), Good: 380, Scrap: 20},
	}
}

var (
	breakEvent = Downtime{Start: at(10, 0), End: at(10, 30), Planned: true}
	lateEvent  = Downtime{Start: at(13, 0), End: at(14, 30), Planned: false}
)

func shift(name string) Shift {
	for _, s := range ShiftsOfDay(at(0, 0)) {
		if s.Name == name {
			return s
		}
	}
	panic(name)
}

func TestShiftsOfDay(t *testing.T) {
	shifts := ShiftsOfDay(at(0, 0))
	if len(shifts) != 3 || !shifts[0].Start.Equal(at(6, 0)) || !shifts[1].Start.Equal(at(14, 0)) ||
		!shifts[2].Start.Equal(at(22, 0)) || !shifts[2].End.Equal(at(30, 0)) {
		t.Fatalf("beklenmeyen vardiyalar: %+v", shifts)
	}
}

func TestCompute_BeforeLateEvent(t *testing.T) {
	a := Compute(shift("A").Start, shift("A").End, time.Minute, []Downtime{breakEvent}, exampleCounts())
	if a.PlannedDowntime != 30*time.Minute || a.UnplannedDowntime != 0 || a.Good != 285 || a.Scrap != 15 {
		t.Fatalf("beklenmeyen A: %+v", a)
	}
	near(t, "A kullanılabilirlik", a.Availability, 1)
	near(t, "A performans", a.Performance, 300.0/450)
	near(t, "A kalite", a.Quality, 0.95)
	near(t, "A OEE", a.OEE, 285.0/450)

	b := Compute(shift("B").Start, shift("B").End, time.Minute, []Downtime{breakEvent}, exampleCounts())
	near(t, "B performans", b.Performance, 400.0/480)
	near(t, "B OEE", b.OEE, 380.0/480)
}

func TestCompute_LateEventUpdatesHistoryAndSplitsAcrossShifts(t *testing.T) {
	events := []Downtime{breakEvent, lateEvent}

	a := Compute(shift("A").Start, shift("A").End, time.Minute, events, exampleCounts())
	if a.UnplannedDowntime != 60*time.Minute || a.RunTime != 390*time.Minute {
		t.Fatalf("A'ya 60 dk plansız duruş düşmeli: %+v", a)
	}
	near(t, "A kullanılabilirlik", a.Availability, 390.0/450)
	near(t, "A performans", a.Performance, 300.0/390)
	near(t, "A OEE", a.OEE, 285.0/450)

	b := Compute(shift("B").Start, shift("B").End, time.Minute, events, exampleCounts())
	if b.UnplannedDowntime != 30*time.Minute {
		t.Fatalf("B'ye 30 dk plansız duruş düşmeli: %+v", b)
	}
	near(t, "B kullanılabilirlik", b.Availability, 450.0/480)
	near(t, "B performans", b.Performance, 400.0/450)
}

func TestCompute_OrderIndependent(t *testing.T) {
	events := []Downtime{breakEvent, lateEvent}
	reversedEvents := []Downtime{lateEvent, breakEvent}
	counts := exampleCounts()
	reversedCounts := []Count{counts[2], counts[1], counts[0]}

	first := Compute(at(6, 0), at(30, 0), time.Minute, events, counts)
	second := Compute(at(6, 0), at(30, 0), time.Minute, reversedEvents, reversedCounts)
	if first != second {
		t.Fatalf("sıra sonucu değiştirmemeli:\n%+v\n%+v", first, second)
	}
}

func TestCompute_OverlappingUnplannedNotDoubleCounted(t *testing.T) {
	events := []Downtime{
		{Start: at(10, 0), End: at(11, 0)},
		{Start: at(10, 30), End: at(11, 30)},
	}
	r := Compute(at(6, 0), at(14, 0), time.Minute, events, nil)
	if r.UnplannedDowntime != 90*time.Minute {
		t.Fatalf("90 dk beklenirdi, gelen %v", r.UnplannedDowntime)
	}
}

func TestCompute_PlannedWinsOverUnplannedOverlap(t *testing.T) {
	events := []Downtime{
		{Start: at(10, 0), End: at(11, 0), Planned: true},
		{Start: at(10, 30), End: at(11, 30)},
	}
	r := Compute(at(6, 0), at(14, 0), time.Minute, events, nil)
	if r.PlannedDowntime != 60*time.Minute || r.UnplannedDowntime != 30*time.Minute {
		t.Fatalf("planlı 60, plansız 30 beklenirdi: %+v", r)
	}
}

func TestCompute_CountOnBoundaryBelongsToNextShift(t *testing.T) {
	counts := []Count{{At: at(14, 0), Good: 10}}
	a := Compute(at(6, 0), at(14, 0), time.Minute, nil, counts)
	b := Compute(at(14, 0), at(22, 0), time.Minute, nil, counts)
	if a.Good != 0 || b.Good != 10 {
		t.Fatalf("sınırdaki sayaç B'ye ait olmalı: A %d, B %d", a.Good, b.Good)
	}
}

func TestCompute_NoProductionOrFullyPlannedIsZeroNotNaN(t *testing.T) {
	empty := Compute(at(6, 0), at(14, 0), time.Minute, nil, nil)
	full := Compute(at(6, 0), at(14, 0), time.Minute, []Downtime{{Start: at(6, 0), End: at(14, 0), Planned: true}}, nil)
	for _, r := range []Result{empty, full} {
		for _, v := range []float64{r.Availability, r.Performance, r.Quality, r.OEE} {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				t.Fatalf("NaN/Inf olmamalı: %+v", r)
			}
		}
		if r.OEE != 0 {
			t.Errorf("OEE 0 olmalı, gelen %v", r.OEE)
		}
	}
}

// --- Servis testleri ---

type fakeRepo struct {
	line      Line
	downtimes []Downtime
	counts    []Count
	calls     int
}

func (f *fakeRepo) CreateLine(_ context.Context, code, name string, cycle time.Duration) (Line, error) {
	return Line{ID: 1, Code: code, Name: name, IdealCycle: cycle}, nil
}
func (f *fakeRepo) ListLines(context.Context) ([]Line, error) { return []Line{f.line}, nil }
func (f *fakeRepo) FindLine(_ context.Context, code string) (Line, error) {
	f.calls++
	if code != f.line.Code {
		return Line{}, ErrLineNotFound
	}
	return f.line, nil
}
func (f *fakeRepo) Reasons(context.Context) ([]Reason, error) { return nil, nil }
func (f *fakeRepo) AddDowntime(_ context.Context, _ int64, reason string, start, end time.Time, _ int64) (int64, error) {
	f.downtimes = append(f.downtimes, Downtime{Start: start, End: end, Planned: reason == "BREAK"})
	return int64(len(f.downtimes)), nil
}
func (f *fakeRepo) AddCount(_ context.Context, _ int64, at time.Time, good, scrap, _ int64) (int64, error) {
	f.counts = append(f.counts, Count{At: at, Good: good, Scrap: scrap})
	return int64(len(f.counts)), nil
}
func (f *fakeRepo) Events(context.Context, int64, time.Time, time.Time) ([]Downtime, []Count, error) {
	return f.downtimes, f.counts, nil
}

func newService() (*Service, *fakeRepo) {
	repo := &fakeRepo{line: Line{ID: 1, Code: "L1", IdealCycle: time.Minute}}
	svc := NewService(repo)
	svc.now = func() time.Time { return at(15, 0) }
	return svc, repo
}

func TestRecordDowntime_Validation(t *testing.T) {
	cases := map[string][2]time.Time{
		"bitiş başlangıçtan önce": {at(10, 0), at(9, 0)},
		"24 saatten uzun":         {at(-20, 0), at(10, 0)},
		"gelecekte bitiyor":       {at(14, 0), at(16, 0)},
		"başlangıç yok":           {{}, at(10, 0)},
	}
	for name, window := range cases {
		t.Run(name, func(t *testing.T) {
			svc, repo := newService()
			if _, err := svc.RecordDowntime(context.Background(), "L1", "BREAKDOWN", window[0], window[1], 5); !errors.Is(err, ErrInvalid) {
				t.Fatalf("ErrInvalid beklenirdi, gelen %v", err)
			}
			if repo.calls != 0 {
				t.Error("geçersiz istekte depo çağrılmamalı")
			}
		})
	}
}

func TestRecordDowntime_UnknownLine(t *testing.T) {
	svc, _ := newService()
	if _, err := svc.RecordDowntime(context.Background(), "YOK", "BREAKDOWN", at(9, 0), at(10, 0), 5); !errors.Is(err, ErrLineNotFound) {
		t.Fatalf("ErrLineNotFound beklenirdi, gelen %v", err)
	}
}

func TestRecordCount_Validation(t *testing.T) {
	svc, _ := newService()
	for name, q := range map[string][2]int64{"negatif": {-1, 0}, "sıfır": {0, 0}, "çok büyük": {MaxQuantity + 1, 0}} {
		if _, err := svc.RecordCount(context.Background(), "L1", at(9, 0), q[0], q[1], 5); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: ErrInvalid beklenirdi, gelen %v", name, err)
		}
	}
}

func TestCreateLine_Validation(t *testing.T) {
	svc, _ := newService()
	if _, err := svc.CreateLine(context.Background(), " ", "Hat", time.Minute); !errors.Is(err, ErrInvalid) {
		t.Errorf("boş kod reddedilmeli, gelen %v", err)
	}
	if _, err := svc.CreateLine(context.Background(), "L2", "Hat", 0); !errors.Is(err, ErrInvalid) {
		t.Errorf("sıfır çevrim reddedilmeli, gelen %v", err)
	}
}

// Geç gelen duruş, yazıldığı anda geçmiş vardiyanın raporunu değiştirmelidir.
func TestDayReport_LateEventChangesPastShift(t *testing.T) {
	svc, _ := newService()
	ctx := context.Background()
	if _, err := svc.RecordDowntime(ctx, "L1", "BREAK", at(10, 0), at(10, 30), 5); err != nil {
		t.Fatal(err)
	}
	for _, c := range exampleCounts()[:2] {
		if _, err := svc.RecordCount(ctx, "L1", c.At, c.Good, c.Scrap, 5); err != nil {
			t.Fatal(err)
		}
	}

	before, err := svc.DayReport(ctx, "L1", at(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	near(t, "önce A kullanılabilirlik", before.Shifts[0].Availability, 1)

	// 14:00'te biten vardiyaya ait duruş, 15:00'te (geç) kaydedilir.
	if _, err := svc.RecordDowntime(ctx, "L1", "BREAKDOWN", at(13, 0), at(14, 30), 6); err != nil {
		t.Fatal(err)
	}

	after, err := svc.DayReport(ctx, "L1", at(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	near(t, "sonra A kullanılabilirlik", after.Shifts[0].Availability, 390.0/450)
	if after.Shifts[1].UnplannedDowntime != 30*time.Minute {
		t.Errorf("B'ye 30 dk düşmeli, gelen %v", after.Shifts[1].UnplannedDowntime)
	}
	if len(after.Shifts) != 3 || after.Shifts[0].Name != "A" {
		t.Errorf("üç vardiya beklenirdi: %+v", after.Shifts)
	}
}
