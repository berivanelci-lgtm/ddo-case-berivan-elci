package ncr

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func newNCR(t *testing.T) NCR {
	t.Helper()
	n, err := New("  Flanşta çapak  ", "kontrolde bulundu", SeverityMajor, nil, nil, 5)
	if err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}
	return n
}

func TestNew_StartsOpenAndTrims(t *testing.T) {
	n := newNCR(t)
	if n.Status != StatusOpen || n.Title != "Flanşta çapak" || n.OpenedBy != 5 {
		t.Fatalf("beklenmeyen kayıt: %+v", n)
	}
}

func TestNew_Rejects(t *testing.T) {
	zero := int64(0)
	cases := []struct {
		name     string
		title    string
		desc     string
		severity Severity
		wo       *int64
	}{
		{"boş başlık", "  ", "", SeverityMinor, nil},
		{"uzun başlık", strings.Repeat("a", 201), "", SeverityMinor, nil},
		{"uzun açıklama", "x", strings.Repeat("a", 4001), SeverityMinor, nil},
		{"bilinmeyen önem", "x", "", Severity("high"), nil},
		{"sıfır iş emri", "x", "", SeverityMinor, &zero},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := New(tc.title, tc.desc, tc.severity, tc.wo, nil, 1); !errors.Is(err, ErrInvalid) {
				t.Fatalf("ErrInvalid beklenirdi, gelen %v", err)
			}
		})
	}
}

func TestLifecycle_OpenInvestigateClose(t *testing.T) {
	at := time.Date(2026, 10, 26, 20, 0, 0, 0, time.UTC)

	investigating, err := newNCR(t).StartInvestigation()
	if err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}
	closed, err := investigating.Close(" takım aşınması ", " takım değiştirildi ", 9, at)
	if err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}
	if closed.Status != StatusClosed || closed.RootCause != "takım aşınması" ||
		closed.CorrectiveAction != "takım değiştirildi" || *closed.ClosedBy != 9 || !closed.ClosedAt.Equal(at) {
		t.Fatalf("beklenmeyen kapanış: %+v", closed)
	}
}

func TestClose_RequiresRootCauseAndAction(t *testing.T) {
	investigating, _ := newNCR(t).StartInvestigation()
	for _, pair := range [][2]string{{"", "x"}, {"x", ""}, {"  ", "  "}} {
		if _, err := investigating.Close(pair[0], pair[1], 1, time.Now()); !errors.Is(err, ErrInvalid) {
			t.Errorf("%q/%q için ErrInvalid beklenirdi, gelen %v", pair[0], pair[1], err)
		}
	}
}

func TestClose_OnlyFromInvestigating(t *testing.T) {
	if _, err := newNCR(t).Close("x", "y", 1, time.Now()); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("açık kayıt doğrudan kapatılamamalı, gelen %v", err)
	}
}

func TestClosedIsFinal(t *testing.T) {
	investigating, _ := newNCR(t).StartInvestigation()
	closed, _ := investigating.Close("x", "y", 1, time.Now())

	if _, err := closed.StartInvestigation(); !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("kapalı kayıt incelemeye alınamamalı, gelen %v", err)
	}
	if _, err := closed.Close("x", "y", 1, time.Now()); !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("kapalı kayıt yeniden kapatılamamalı, gelen %v", err)
	}
}

func TestInvestigate_OnlyFromOpen(t *testing.T) {
	investigating, _ := newNCR(t).StartInvestigation()
	if _, err := investigating.StartInvestigation(); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("ErrInvalidTransition beklenirdi, gelen %v", err)
	}
}
