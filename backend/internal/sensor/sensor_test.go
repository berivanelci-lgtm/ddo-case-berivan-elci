package sensor

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"
)

var now = time.Date(2026, 10, 26, 20, 0, 0, 0, time.UTC)

func valid() Reading { return Reading{StationID: 7, RecordedAt: now.Add(-time.Second), Value: 50.2} }

func TestValidate_AcceptsBatch(t *testing.T) {
	if err := Validate([]Reading{valid(), valid()}, now); err != nil {
		t.Fatalf("geçerli toplu okuma kabul edilmeli: %v", err)
	}
}

func TestValidate_Rejects(t *testing.T) {
	big := make([]Reading, MaxBatch+1)
	for i := range big {
		big[i] = valid()
	}
	mutate := func(change func(*Reading)) []Reading {
		r := valid()
		change(&r)
		return []Reading{r}
	}
	cases := map[string][]Reading{
		"boş":            nil,
		"çok büyük":      big,
		"istasyon sıfır": mutate(func(r *Reading) { r.StationID = 0 }),
		"istasyon büyük": mutate(func(r *Reading) { r.StationID = 1001 }),
		"zaman yok":      mutate(func(r *Reading) { r.RecordedAt = time.Time{} }),
		"gelecek":        mutate(func(r *Reading) { r.RecordedAt = now.Add(time.Hour) }),
		"çok eski":       mutate(func(r *Reading) { r.RecordedAt = now.Add(-91 * 24 * time.Hour) }),
		"NaN":            mutate(func(r *Reading) { r.Value = math.NaN() }),
	}
	for name, readings := range cases {
		t.Run(name, func(t *testing.T) {
			if err := Validate(readings, now); !errors.Is(err, ErrInvalid) {
				t.Fatalf("ErrInvalid beklenirdi, gelen %v", err)
			}
		})
	}
}

func TestMonths_DistinctAndSorted(t *testing.T) {
	readings := []Reading{
		{RecordedAt: time.Date(2026, 10, 31, 23, 59, 0, 0, time.UTC)},
		{RecordedAt: time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC)},
		{RecordedAt: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)},
	}
	months := Months(readings)
	if len(months) != 2 || !months[0].Equal(time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)) ||
		!months[1].Equal(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("eylül ve ekim beklenirdi, gelen %v", months)
	}
}

type fakeStore struct{ calls int }

func (f *fakeStore) Insert(_ context.Context, readings []Reading) (int64, error) {
	f.calls++
	return int64(len(readings)), nil
}

func TestIngest_ValidatesBeforeWriting(t *testing.T) {
	store := &fakeStore{}
	svc := NewService(store)
	svc.now = func() time.Time { return now }

	if _, err := svc.Ingest(context.Background(), nil); !errors.Is(err, ErrInvalid) || store.calls != 0 {
		t.Fatalf("geçersiz toplu okuma yazılmamalı: %v, çağrı %d", err, store.calls)
	}
	n, err := svc.Ingest(context.Background(), []Reading{valid(), valid(), valid()})
	if err != nil || n != 3 {
		t.Fatalf("3 satır yazılmalı: %d %v", n, err)
	}
}
