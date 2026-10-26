// Package oee, OEE (kullanılabilirlik × performans × kalite) hesabını olay kayıtlarından üretir.
// Hesap saf ve deterministiktir: aynı olaylar, hangi sırayla verilirse verilsin, aynı sonucu verir.
package oee

import (
	"sort"
	"time"
)

// Downtime, bir duruş olayıdır.
type Downtime struct {
	Start   time.Time
	End     time.Time
	Planned bool
}

// Count, belirli bir anda kaydedilen üretim sayacıdır.
type Count struct {
	At    time.Time
	Good  int64
	Scrap int64
}

// Result, bir zaman penceresinin (vardiya ya da gün) OEE sonucudur.
type Result struct {
	Start                 time.Time
	End                   time.Time
	PlannedDowntime       time.Duration
	UnplannedDowntime     time.Duration
	PlannedProductionTime time.Duration
	RunTime               time.Duration
	Good                  int64
	Scrap                 int64
	Availability          float64
	Performance           float64
	Quality               float64
	OEE                   float64
}

// Shift, bir vardiyadır.
type Shift struct {
	Name  string
	Start time.Time
	End   time.Time
}

// ShiftsOfDay, verilen günün üç vardiyasını döner (UTC): A 06-14, B 14-22, C 22-06 (ertesi gün).
func ShiftsOfDay(day time.Time) []Shift {
	base := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC)
	return []Shift{
		{Name: "A", Start: base.Add(6 * time.Hour), End: base.Add(14 * time.Hour)},
		{Name: "B", Start: base.Add(14 * time.Hour), End: base.Add(22 * time.Hour)},
		{Name: "C", Start: base.Add(22 * time.Hour), End: base.Add(30 * time.Hour)},
	}
}

// Compute, [start, end) penceresinin OEE değerini hesaplar.
// Duruşlar pencereye kırpılır (vardiya sınırını aşan olay böylece bölünür).
// Sayaçlar, zamanı pencerenin içindeyse sayılır (bitiş anı bir sonraki pencereye aittir).
func Compute(start, end time.Time, idealCycle time.Duration, downtimes []Downtime, counts []Count) Result {
	result := Result{Start: start, End: end}
	result.PlannedDowntime, result.UnplannedDowntime = splitDowntime(start, end, downtimes)
	result.PlannedProductionTime = end.Sub(start) - result.PlannedDowntime
	result.RunTime = result.PlannedProductionTime - result.UnplannedDowntime

	for _, count := range counts {
		if !count.At.Before(start) && count.At.Before(end) {
			result.Good += count.Good
			result.Scrap += count.Scrap
		}
	}
	total := result.Good + result.Scrap

	if result.PlannedProductionTime > 0 {
		result.Availability = float64(result.RunTime) / float64(result.PlannedProductionTime)
	}
	if result.RunTime > 0 {
		result.Performance = float64(total) * float64(idealCycle) / float64(result.RunTime)
	}
	if total > 0 {
		result.Quality = float64(result.Good) / float64(total)
	}
	result.OEE = result.Availability * result.Performance * result.Quality
	return result
}

// splitDowntime, penceredeki planlı ve plansız duruş sürelerini döner.
// Çakışan duruşlar bir kez sayılır; planlı ve plansız çakışırsa o süre planlı sayılır.
func splitDowntime(start, end time.Time, downtimes []Downtime) (planned, unplanned time.Duration) {
	var clipped []Downtime
	var points []time.Time
	for _, d := range downtimes {
		s, e := d.Start, d.End
		if s.Before(start) {
			s = start
		}
		if e.After(end) {
			e = end
		}
		if !s.Before(e) {
			continue
		}
		clipped = append(clipped, Downtime{Start: s, End: e, Planned: d.Planned})
		points = append(points, s, e)
	}
	sort.Slice(points, func(i, j int) bool { return points[i].Before(points[j]) })

	for i := 0; i+1 < len(points); i++ {
		segStart, segEnd := points[i], points[i+1]
		if !segStart.Before(segEnd) {
			continue
		}
		coveredPlanned, coveredUnplanned := false, false
		for _, d := range clipped {
			if !d.Start.After(segStart) && !d.End.Before(segEnd) {
				if d.Planned {
					coveredPlanned = true
				} else {
					coveredUnplanned = true
				}
			}
		}
		switch {
		case coveredPlanned:
			planned += segEnd.Sub(segStart)
		case coveredUnplanned:
			unplanned += segEnd.Sub(segStart)
		}
	}
	return planned, unplanned
}
