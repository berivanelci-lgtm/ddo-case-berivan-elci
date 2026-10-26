// Package ncr, uygunsuzluk kayıtlarının (NCR) durum makinesini ve doğrulamasını içerir.
package ncr

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	maxTitleLength = 200
	maxTextLength  = 4000
)

// Severity, uygunsuzluğun önem derecesidir.
type Severity string

const (
	SeverityMinor    Severity = "minor"
	SeverityMajor    Severity = "major"
	SeverityCritical Severity = "critical"
)

// Valid, önem derecesinin tanımlı olup olmadığını söyler.
func (s Severity) Valid() bool {
	return s == SeverityMinor || s == SeverityMajor || s == SeverityCritical
}

// Status, kaydın durumudur.
type Status string

const (
	StatusOpen          Status = "open"
	StatusInvestigating Status = "investigating"
	StatusClosed        Status = "closed"
)

// Valid, durumun tanımlı olup olmadığını söyler.
func (s Status) Valid() bool {
	return s == StatusOpen || s == StatusInvestigating || s == StatusClosed
}

var (
	// ErrInvalid, girilen değerler geçersiz olduğunda döner.
	ErrInvalid = errors.New("uygunsuzluk kaydı geçersiz")
	// ErrInvalidTransition, işlem kaydın mevcut durumunda yapılamıyorsa döner.
	ErrInvalidTransition = errors.New("bu durumda bu işlem yapılamaz")
	// ErrNotFound, kayıt bulunamadığında döner.
	ErrNotFound = errors.New("uygunsuzluk kaydı bulunamadı")
)

// NCR, bir uygunsuzluk kaydıdır. Metotlar değer alıcıdır ve yeni durumu döner.
type NCR struct {
	ID               int64
	Title            string
	Description      string
	Severity         Severity
	Status           Status
	WorkOrderID      *int64
	ChartID          *int64
	OpenedBy         int64
	RootCause        string
	CorrectiveAction string
	ClosedBy         *int64
	CreatedAt        time.Time
	ClosedAt         *time.Time
}

// New, açık durumda yeni bir uygunsuzluk kaydı oluşturur.
func New(title, description string, severity Severity, workOrderID, chartID *int64, openedBy int64) (NCR, error) {
	cleanTitle := strings.TrimSpace(title)
	if cleanTitle == "" || utf8.RuneCountInString(cleanTitle) > maxTitleLength {
		return NCR{}, fmt.Errorf("%w: başlık 1 ile %d karakter arasında olmalı", ErrInvalid, maxTitleLength)
	}
	cleanDescription := strings.TrimSpace(description)
	if utf8.RuneCountInString(cleanDescription) > maxTextLength {
		return NCR{}, fmt.Errorf("%w: açıklama en fazla %d karakter olmalı", ErrInvalid, maxTextLength)
	}
	if !severity.Valid() {
		return NCR{}, fmt.Errorf("%w: önem derecesi minor, major ya da critical olmalı", ErrInvalid)
	}
	for _, id := range []*int64{workOrderID, chartID} {
		if id != nil && *id <= 0 {
			return NCR{}, fmt.Errorf("%w: bağlı kayıt kimliği pozitif olmalı", ErrInvalid)
		}
	}

	return NCR{
		Title:       cleanTitle,
		Description: cleanDescription,
		Severity:    severity,
		Status:      StatusOpen,
		WorkOrderID: workOrderID,
		ChartID:     chartID,
		OpenedBy:    openedBy,
	}, nil
}

// StartInvestigation, açık kaydı incelemeye alır.
func (n NCR) StartInvestigation() (NCR, error) {
	if n.Status != StatusOpen {
		return NCR{}, fmt.Errorf("%w: %q durumundaki kayıt incelemeye alınamaz", ErrInvalidTransition, n.Status)
	}
	n.Status = StatusInvestigating
	return n, nil
}

// Close, incelemedeki kaydı kök neden ve düzeltici faaliyetle kapatır.
func (n NCR) Close(rootCause, correctiveAction string, closedBy int64, at time.Time) (NCR, error) {
	if n.Status != StatusInvestigating {
		return NCR{}, fmt.Errorf("%w: yalnızca incelemedeki kayıt kapatılabilir, durum %q", ErrInvalidTransition, n.Status)
	}
	cause := strings.TrimSpace(rootCause)
	action := strings.TrimSpace(correctiveAction)
	if cause == "" || action == "" {
		return NCR{}, fmt.Errorf("%w: kapatmak için kök neden ve düzeltici faaliyet zorunludur", ErrInvalid)
	}
	if utf8.RuneCountInString(cause) > maxTextLength || utf8.RuneCountInString(action) > maxTextLength {
		return NCR{}, fmt.Errorf("%w: metinler en fazla %d karakter olmalı", ErrInvalid, maxTextLength)
	}

	closedAt := at
	n.Status = StatusClosed
	n.RootCause = cause
	n.CorrectiveAction = action
	n.ClosedBy = &closedBy
	n.ClosedAt = &closedAt
	return n, nil
}

// Repository, uygunsuzluk kayıtlarının saklandığı yerdir.
type Repository interface {
	Create(ctx context.Context, n NCR) (NCR, error)
	Get(ctx context.Context, id int64) (NCR, error)
	// List, kayıtları yeniden eskiye sıralı döner. status boşsa hepsi gelir (keyset sayfalama).
	List(ctx context.Context, status Status, before int64, limit int) ([]NCR, error)
	// Update, kaydı kilitler, change ile değiştirir ve kaydeder; hepsi tek işlemde olur.
	Update(ctx context.Context, id int64, change func(NCR) (NCR, error)) (NCR, error)
}
