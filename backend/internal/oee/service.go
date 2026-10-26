package oee

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	// MaxQuantity, tek sayaç kaydında kabul edilen en büyük adettir.
	MaxQuantity    int64 = 1_000_000_000
	maxEventLength       = 24 * time.Hour
	clockSkew            = 5 * time.Minute
)

var (
	ErrInvalid       = errors.New("geçersiz değer")
	ErrLineNotFound  = errors.New("hat bulunamadı")
	ErrLineExists    = errors.New("bu kodla bir hat zaten var")
	ErrUnknownReason = errors.New("duruş nedeni tanımlı değil")
)

// Line, bir üretim hattıdır.
type Line struct {
	ID         int64
	Code       string
	Name       string
	IdealCycle time.Duration
}

// Reason, bir duruş nedenidir; planlı olup olmadığı nedene bağlıdır.
type Reason struct {
	Code        string
	Description string
	Planned     bool
}

// Repository, hat, duruş ve sayaç kayıtlarının saklandığı yerdir.
type Repository interface {
	CreateLine(ctx context.Context, code, name string, idealCycle time.Duration) (Line, error)
	ListLines(ctx context.Context) ([]Line, error)
	FindLine(ctx context.Context, code string) (Line, error)
	Reasons(ctx context.Context) ([]Reason, error)
	AddDowntime(ctx context.Context, lineID int64, reasonCode string, start, end time.Time, userID int64) (int64, error)
	AddCount(ctx context.Context, lineID int64, at time.Time, good, scrap, userID int64) (int64, error)
	// Events, [from, to) penceresiyle kesişen duruşları ve penceredeki sayaçları döner.
	Events(ctx context.Context, lineID int64, from, to time.Time) ([]Downtime, []Count, error)
}

// ShiftResult, bir vardiyanın sonucudur.
type ShiftResult struct {
	Name string
	Result
}

// DayReport, bir hattın bir günlük (üç vardiya) OEE raporudur.
type DayReport struct {
	Line   Line
	Day    time.Time
	Shifts []ShiftResult
	Total  Result
}

// Service, OEE işlemlerini yürütür.
type Service struct {
	repo Repository
	now  func() time.Time
}

// NewService, yeni bir Service oluşturur.
func NewService(repo Repository) *Service {
	return &Service{repo: repo, now: time.Now}
}

// CreateLine, yeni bir hat tanımlar.
func (s *Service) CreateLine(ctx context.Context, code, name string, idealCycle time.Duration) (Line, error) {
	cleanCode, cleanName := strings.TrimSpace(code), strings.TrimSpace(name)
	if cleanCode == "" || utf8.RuneCountInString(cleanCode) > 40 {
		return Line{}, fmt.Errorf("%w: kod 1 ile 40 karakter arasında olmalı", ErrInvalid)
	}
	if cleanName == "" || utf8.RuneCountInString(cleanName) > 100 {
		return Line{}, fmt.Errorf("%w: ad 1 ile 100 karakter arasında olmalı", ErrInvalid)
	}
	if idealCycle < time.Millisecond || idealCycle > time.Hour {
		return Line{}, fmt.Errorf("%w: ideal çevrim süresi 1 ms ile 1 saat arasında olmalı", ErrInvalid)
	}
	return s.repo.CreateLine(ctx, cleanCode, cleanName, idealCycle)
}

// ListLines, hatları döner.
func (s *Service) ListLines(ctx context.Context) ([]Line, error) { return s.repo.ListLines(ctx) }

// Reasons, duruş nedenlerini döner.
func (s *Service) Reasons(ctx context.Context) ([]Reason, error) { return s.repo.Reasons(ctx) }

// RecordDowntime, bir duruş olayını kaydeder. Olay geç gelebilir; geçmiş vardiyaların sonucu
// her sorguda olaylardan yeniden hesaplandığı için otomatik düzelir.
func (s *Service) RecordDowntime(ctx context.Context, lineCode, reasonCode string, start, end time.Time, userID int64) (int64, error) {
	reason := strings.TrimSpace(reasonCode)
	switch {
	case start.IsZero() || end.IsZero():
		return 0, fmt.Errorf("%w: started_at ve ended_at zorunludur", ErrInvalid)
	case !end.After(start):
		return 0, fmt.Errorf("%w: bitiş başlangıçtan sonra olmalı", ErrInvalid)
	case end.Sub(start) > maxEventLength:
		return 0, fmt.Errorf("%w: bir duruş en fazla 24 saat olabilir", ErrInvalid)
	case end.After(s.now().Add(clockSkew)):
		return 0, fmt.Errorf("%w: gelecekte biten bir duruş kaydedilemez", ErrInvalid)
	case reason == "":
		return 0, fmt.Errorf("%w: reason_code zorunludur", ErrInvalid)
	}

	line, err := s.repo.FindLine(ctx, strings.TrimSpace(lineCode))
	if err != nil {
		return 0, err
	}
	return s.repo.AddDowntime(ctx, line.ID, reason, start.UTC(), end.UTC(), userID)
}

// RecordCount, bir üretim sayacı kaydeder.
func (s *Service) RecordCount(ctx context.Context, lineCode string, at time.Time, good, scrap, userID int64) (int64, error) {
	switch {
	case at.IsZero():
		return 0, fmt.Errorf("%w: occurred_at zorunludur", ErrInvalid)
	case at.After(s.now().Add(clockSkew)):
		return 0, fmt.Errorf("%w: gelecekteki bir zaman kaydedilemez", ErrInvalid)
	case good < 0 || scrap < 0 || good > MaxQuantity || scrap > MaxQuantity:
		return 0, fmt.Errorf("%w: adetler 0 ile %d arasında olmalı", ErrInvalid, MaxQuantity)
	case good+scrap == 0:
		return 0, fmt.Errorf("%w: en az bir ürün kaydedilmeli", ErrInvalid)
	}

	line, err := s.repo.FindLine(ctx, strings.TrimSpace(lineCode))
	if err != nil {
		return 0, err
	}
	return s.repo.AddCount(ctx, line.ID, at.UTC(), good, scrap, userID)
}

// DayReport, hattın verilen gündeki üç vardiyasının ve günün toplamının OEE değerini olaylardan hesaplar.
func (s *Service) DayReport(ctx context.Context, lineCode string, day time.Time) (DayReport, error) {
	line, err := s.repo.FindLine(ctx, strings.TrimSpace(lineCode))
	if err != nil {
		return DayReport{}, err
	}

	shifts := ShiftsOfDay(day)
	from, to := shifts[0].Start, shifts[len(shifts)-1].End
	downtimes, counts, err := s.repo.Events(ctx, line.ID, from, to)
	if err != nil {
		return DayReport{}, err
	}

	report := DayReport{
		Line: line,
		Day:  time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC),
	}
	for _, shift := range shifts {
		report.Shifts = append(report.Shifts, ShiftResult{
			Name:   shift.Name,
			Result: Compute(shift.Start, shift.End, line.IdealCycle, downtimes, counts),
		})
	}
	report.Total = Compute(from, to, line.IdealCycle, downtimes, counts)
	return report, nil
}
