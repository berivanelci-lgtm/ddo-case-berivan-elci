package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/auth"
	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/oee"
	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/user"
)

type fakeOEEService struct {
	report   oee.DayReport
	err      error
	gotUser  int64
	gotStart time.Time
	gotLine  string
	gotDay   time.Time
}

func (f *fakeOEEService) CreateLine(_ context.Context, code, name string, cycle time.Duration) (oee.Line, error) {
	return oee.Line{ID: 1, Code: code, Name: name, IdealCycle: cycle}, f.err
}
func (f *fakeOEEService) ListLines(context.Context) ([]oee.Line, error) { return nil, f.err }
func (f *fakeOEEService) Reasons(context.Context) ([]oee.Reason, error) {
	return []oee.Reason{{Code: "BREAK", Description: "Mola", Planned: true}}, f.err
}
func (f *fakeOEEService) RecordDowntime(_ context.Context, line, _ string, start, _ time.Time, userID int64) (int64, error) {
	f.gotLine, f.gotStart, f.gotUser = line, start, userID
	return 7, f.err
}
func (f *fakeOEEService) RecordCount(_ context.Context, line string, _ time.Time, _, _, userID int64) (int64, error) {
	f.gotLine, f.gotUser = line, userID
	return 8, f.err
}
func (f *fakeOEEService) DayReport(_ context.Context, line string, day time.Time) (oee.DayReport, error) {
	f.gotLine, f.gotDay = line, day
	return f.report, f.err
}

type linesEnv struct {
	handler http.Handler
	tokens  *auth.TokenManager
	service *fakeOEEService
}

func newLinesEnv(t *testing.T) *linesEnv {
	t.Helper()
	tokens := newTestTokens(t)
	service := &fakeOEEService{}
	mux := http.NewServeMux()
	NewLinesAPI(service).Register(mux, Authenticate(tokens))
	return &linesEnv{handler: mux, tokens: tokens, service: service}
}

func (e *linesEnv) call(t *testing.T, role user.Role, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if role != "" {
		req.Header.Set("Authorization", "Bearer "+issueToken(t, e.tokens, role, auth.AccessToken))
	}
	rec := httptest.NewRecorder()
	e.handler.ServeHTTP(rec, req)
	return rec
}

const (
	downtimeBody = `{"reason_code":"BREAKDOWN","started_at":"2026-10-20T13:00:00Z","ended_at":"2026-10-20T14:30:00Z"}`
	countBody    = `{"occurred_at":"2026-10-20T08:00:00Z","good":10,"scrap":1}`
	lineBody     = `{"code":"L1","name":"Hat 1","ideal_cycle_ms":60000}`
)

func TestLines_RequireAuthentication(t *testing.T) {
	if rec := newLinesEnv(t).call(t, "", "GET", "/lines/L1/oee?date=2026-10-20", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("401 beklenirdi, gelen %d", rec.Code)
	}
}

func TestLines_RoleMatrix(t *testing.T) {
	cases := []struct {
		name         string
		role         user.Role
		method, path string
		body         string
		want         int
	}{
		{"planlama hat açar", user.RoleUretimPlanlama, "POST", "/lines", lineBody, 201},
		{"operatör hat açamaz", user.RoleOperator, "POST", "/lines", lineBody, 403},
		{"bakım duruş girer", user.RoleBakim, "POST", "/lines/L1/downtime", downtimeBody, 201},
		{"kalite duruş giremez", user.RoleKaliteMuhendisi, "POST", "/lines/L1/downtime", downtimeBody, 403},
		{"operatör sayaç girer", user.RoleOperator, "POST", "/lines/L1/counts", countBody, 201},
		{"bakım sayaç giremez", user.RoleBakim, "POST", "/lines/L1/counts", countBody, 403},
		{"müdür OEE okur", user.RoleFabrikaMuduru, "GET", "/lines/L1/oee?date=2026-10-20", "", 200},
		{"herkes nedenleri okur", user.RoleOperator, "GET", "/downtime-reasons", "", 200},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if rec := newLinesEnv(t).call(t, tc.role, tc.method, tc.path, tc.body); rec.Code != tc.want {
				t.Fatalf("%d beklenirdi, gelen %d (gövde: %s)", tc.want, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestDowntime_PassesUserAndTime(t *testing.T) {
	env := newLinesEnv(t)
	env.call(t, user.RoleBakim, "POST", "/lines/L1/downtime", downtimeBody)

	if env.service.gotLine != "L1" || env.service.gotUser != 5 ||
		!env.service.gotStart.Equal(time.Date(2026, 10, 20, 13, 0, 0, 0, time.UTC)) {
		t.Fatalf("servise yanlış değerler gitti: %+v", env.service)
	}
}

func TestOEE_InvalidDate(t *testing.T) {
	for _, query := range []string{"", "?date=20-10-2026", "?date=abc"} {
		if rec := newLinesEnv(t).call(t, user.RoleHatSefi, "GET", "/lines/L1/oee"+query, ""); rec.Code != http.StatusBadRequest {
			t.Errorf("%q için 400 beklenirdi, gelen %d", query, rec.Code)
		}
	}
}

func TestOEE_ResponseRoundsAndConvertsMinutes(t *testing.T) {
	env := newLinesEnv(t)
	result := oee.Result{
		UnplannedDowntime: 60 * time.Minute, PlannedProductionTime: 450 * time.Minute, RunTime: 390 * time.Minute,
		Good: 285, Scrap: 15, Availability: 390.0 / 450, Performance: 300.0 / 390, Quality: 0.95, OEE: 285.0 / 450,
	}
	env.service.report = oee.DayReport{
		Line:   oee.Line{ID: 1, Code: "L1", IdealCycle: time.Minute},
		Day:    time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC),
		Shifts: []oee.ShiftResult{{Name: "A", Result: result}},
		Total:  result,
	}

	rec := env.call(t, user.RoleHatSefi, "GET", "/lines/L1/oee?date=2026-10-20", "")

	var resp oeeReportResponse
	decodeInto(t, rec, &resp)
	a := resp.Shifts[0]
	if a.Shift != "A" || a.UnplannedDowntimeMin != 60 || a.Availability != 0.8667 || a.Performance != 0.7692 || a.OEE != 0.6333 {
		t.Fatalf("beklenmeyen yanıt: %+v", a)
	}
	if resp.Date != "2026-10-20" || resp.Line.IdealCycleMS != 60000 || !env.service.gotDay.Equal(time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("beklenmeyen tarih ya da hat: %+v", resp)
	}
}

func TestLines_ErrorMapping(t *testing.T) {
	cases := []struct {
		err    error
		status int
		code   string
	}{
		{oee.ErrInvalid, 400, "validation_error"},
		{oee.ErrLineNotFound, 404, "line_not_found"},
		{oee.ErrUnknownReason, 422, "unknown_reason"},
		{errors.New("bağlantı koptu: 10.0.0.15"), 500, "internal_error"},
	}
	for _, tc := range cases {
		env := newLinesEnv(t)
		env.service.err = tc.err

		rec := env.call(t, user.RoleBakim, "POST", "/lines/L1/downtime", downtimeBody)
		if rec.Code != tc.status || decodeError(t, rec).Error.Code != tc.code {
			t.Errorf("%v: %d/%s beklenirdi, gelen %d %s", tc.err, tc.status, tc.code, rec.Code, rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), "10.0.0.15") {
			t.Error("iç hata sızmamalı")
		}
	}
}

func TestLines_BadTimeFormatIsInvalidRequest(t *testing.T) {
	body := `{"reason_code":"BREAKDOWN","started_at":"dün","ended_at":"bugün"}`
	if rec := newLinesEnv(t).call(t, user.RoleBakim, "POST", "/lines/L1/downtime", body); rec.Code != http.StatusBadRequest {
		t.Fatalf("400 beklenirdi, gelen %d", rec.Code)
	}
}
