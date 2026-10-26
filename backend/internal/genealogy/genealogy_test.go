package genealogy

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeRepo struct {
	lots      map[string]Lot
	backward  []Node
	forward   []Node
	shipments []Shipment
	err       error

	findCalls int
	gotDepth  int
}

func (f *fakeRepo) FindLot(_ context.Context, code string) (Lot, error) {
	f.findCalls++
	lot, ok := f.lots[code]
	if !ok {
		return Lot{}, ErrLotNotFound
	}
	return lot, nil
}

func (f *fakeRepo) Backward(_ context.Context, _ int64, maxDepth int) ([]Node, error) {
	f.gotDepth = maxDepth
	return f.backward, f.err
}

func (f *fakeRepo) Forward(_ context.Context, _ int64, maxDepth int) ([]Node, error) {
	f.gotDepth = maxDepth
	return f.forward, f.err
}

func (f *fakeRepo) Shipments(context.Context, int64, int) ([]Shipment, error) {
	return f.shipments, f.err
}

func newRepo() *fakeRepo {
	return &fakeRepo{lots: map[string]Lot{
		"RAW-1": {ID: 1, Code: "RAW-1", Kind: KindRaw},
		"SN-9":  {ID: 9, Code: "SN-9", Kind: KindSerial},
	}}
}

func TestRecall_CollectsSerialsAndDistinctCustomers(t *testing.T) {
	repo := newRepo()
	repo.forward = []Node{
		{Lot: Lot{ID: 2, Code: "INT-2", Kind: KindIntermediate}, Depth: 1},
		{Lot: Lot{ID: 4, Code: "SN-4", Kind: KindSerial}, Depth: 2},
		{Lot: Lot{ID: 3, Code: "SN-3", Kind: KindSerial}, Depth: 2},
	}
	repo.shipments = []Shipment{
		{Code: "SHP-1", Customer: "B", Units: 1},
		{Code: "SHP-2", Customer: "A", Units: 1},
		{Code: "SHP-3", Customer: "B", Units: 1},
	}

	report, err := NewService(repo).Recall(context.Background(), " RAW-1 ")
	if err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}
	if strings.Join(report.AffectedSerials, ",") != "SN-3,SN-4" {
		t.Errorf("yalnızca seriler sıralı gelmeli, gelen %v", report.AffectedSerials)
	}
	if strings.Join(report.Customers, ",") != "A,B" {
		t.Errorf("müşteriler tekil ve sıralı olmalı, gelen %v", report.Customers)
	}
	if len(report.Shipments) != 3 {
		t.Errorf("3 sevkiyat beklenirdi, gelen %d", len(report.Shipments))
	}
}

func TestRecall_SerialLotIncludesItself(t *testing.T) {
	report, err := NewService(newRepo()).Recall(context.Background(), "SN-9")
	if err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}
	if len(report.AffectedSerials) != 1 || report.AffectedSerials[0] != "SN-9" {
		t.Fatalf("seri kendisi etkilenenler arasında olmalı, gelen %v", report.AffectedSerials)
	}
	if report.Shipments == nil || report.Customers == nil {
		t.Error("boş listeler nil değil boş dizi olmalı")
	}
}

func TestFind_RejectsInvalidCodeWithoutQuery(t *testing.T) {
	for _, code := range []string{"", "   ", strings.Repeat("a", 41)} {
		repo := newRepo()
		_, _, err := NewService(repo).Backward(context.Background(), code)
		if !errors.Is(err, ErrInvalidCode) {
			t.Errorf("%q için ErrInvalidCode beklenirdi, gelen %v", code, err)
		}
		if repo.findCalls != 0 {
			t.Errorf("%q için depo sorgulanmamalı", code)
		}
	}
}

func TestFind_NotFound(t *testing.T) {
	if _, _, err := NewService(newRepo()).Forward(context.Background(), "YOK"); !errors.Is(err, ErrLotNotFound) {
		t.Fatalf("ErrLotNotFound beklenirdi, gelen %v", err)
	}
}

func TestBackward_UsesMaxDepth(t *testing.T) {
	repo := newRepo()
	if _, _, err := NewService(repo).Backward(context.Background(), "SN-9"); err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}
	if repo.gotDepth != MaxDepth {
		t.Errorf("derinlik sınırı %d olmalı, gelen %d", MaxDepth, repo.gotDepth)
	}
}

func TestRecall_PropagatesRepositoryError(t *testing.T) {
	repo := newRepo()
	repo.err = errors.New("veritabanı kapalı")

	if _, err := NewService(repo).Recall(context.Background(), "RAW-1"); err == nil {
		t.Fatal("depo hatası yukarı iletilmeli")
	}
}
