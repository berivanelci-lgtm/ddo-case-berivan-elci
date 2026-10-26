// Package genealogy, parti ve seri numarası soy ağacını (ileri/geri izlenebilirlik)
// ve geri çağırma raporunu yönetir.
package genealogy

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	// MaxDepth, izlemenin inebileceği en fazla seviyedir; döngüye karşı da sınırdır.
	MaxDepth      = 20
	maxCodeLength = 40
)

// Kind, partinin türüdür.
type Kind string

const (
	KindRaw          Kind = "raw"
	KindIntermediate Kind = "intermediate"
	KindSerial       Kind = "serial"
)

var (
	// ErrLotNotFound, parti ya da seri bulunamadığında döner.
	ErrLotNotFound = errors.New("parti ya da seri bulunamadı")
	// ErrInvalidCode, parti kodu geçersiz olduğunda döner.
	ErrInvalidCode = errors.New("parti kodu geçersiz")
)

// Lot, bir hammadde partisi, ara ürün partisi ya da bitmiş ürün seri numarasıdır.
type Lot struct {
	ID   int64
	Code string
	Kind Kind
}

// Node, izlemede bulunan bir partidir; Depth başlangıç partisine olan seviye uzaklığıdır.
type Node struct {
	Lot
	Depth int
}

// Shipment, etkilenen birimleri içeren bir müşteri sevkiyatıdır.
type Shipment struct {
	ID        int64
	Code      string
	Customer  string
	ShippedAt time.Time
	Units     int64 // bu sevkiyattaki etkilenen birim sayısı
}

// Repository, soy ağacı sorgularını yapan depodur.
type Repository interface {
	FindLot(ctx context.Context, code string) (Lot, error)
	Backward(ctx context.Context, lotID int64, maxDepth int) ([]Node, error)
	Forward(ctx context.Context, lotID int64, maxDepth int) ([]Node, error)
	Shipments(ctx context.Context, lotID int64, maxDepth int) ([]Shipment, error)
}

// RecallReport, bir partinin geri çağırılmasından etkilenenleri özetler.
type RecallReport struct {
	Lot             Lot
	AffectedSerials []string
	Shipments       []Shipment
	Customers       []string
}

// Service, izlenebilirlik işlemlerini yürütür.
type Service struct {
	repo Repository
}

// NewService, yeni bir Service oluşturur.
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) find(ctx context.Context, code string) (Lot, error) {
	clean := strings.TrimSpace(code)
	if clean == "" || utf8.RuneCountInString(clean) > maxCodeLength {
		return Lot{}, fmt.Errorf("%w: 1 ile %d karakter arasında olmalı", ErrInvalidCode, maxCodeLength)
	}
	return s.repo.FindLot(ctx, clean)
}

// Backward, bir partinin üretiminde kullanılan tüm partileri döner (geri izleme).
func (s *Service) Backward(ctx context.Context, code string) (Lot, []Node, error) {
	lot, err := s.find(ctx, code)
	if err != nil {
		return Lot{}, nil, err
	}
	nodes, err := s.repo.Backward(ctx, lot.ID, MaxDepth)
	if err != nil {
		return Lot{}, nil, err
	}
	return lot, nodes, nil
}

// Forward, bir partinin kullanıldığı tüm partileri döner (ileri izleme).
func (s *Service) Forward(ctx context.Context, code string) (Lot, []Node, error) {
	lot, err := s.find(ctx, code)
	if err != nil {
		return Lot{}, nil, err
	}
	nodes, err := s.repo.Forward(ctx, lot.ID, MaxDepth)
	if err != nil {
		return Lot{}, nil, err
	}
	return lot, nodes, nil
}

// Recall, partiden etkilenen seri numaralarını, sevkiyatları ve müşterileri döner.
func (s *Service) Recall(ctx context.Context, code string) (RecallReport, error) {
	lot, nodes, err := s.Forward(ctx, code)
	if err != nil {
		return RecallReport{}, err
	}
	shipments, err := s.repo.Shipments(ctx, lot.ID, MaxDepth)
	if err != nil {
		return RecallReport{}, err
	}

	serials := make([]string, 0)
	if lot.Kind == KindSerial {
		serials = append(serials, lot.Code)
	}
	for _, node := range nodes {
		if node.Kind == KindSerial {
			serials = append(serials, node.Code)
		}
	}
	sort.Strings(serials)

	seen := make(map[string]bool)
	customers := make([]string, 0)
	for _, shipment := range shipments {
		if !seen[shipment.Customer] {
			seen[shipment.Customer] = true
			customers = append(customers, shipment.Customer)
		}
	}
	sort.Strings(customers)

	if shipments == nil {
		shipments = []Shipment{}
	}
	return RecallReport{Lot: lot, AffectedSerials: serials, Shipments: shipments, Customers: customers}, nil
}
