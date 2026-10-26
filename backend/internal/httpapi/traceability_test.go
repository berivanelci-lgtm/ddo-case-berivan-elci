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
	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/genealogy"
	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/user"
)

type fakeTraceService struct {
	lot     genealogy.Lot
	nodes   []genealogy.Node
	report  genealogy.RecallReport
	err     error
	gotCode string
}

func (f *fakeTraceService) Backward(_ context.Context, code string) (genealogy.Lot, []genealogy.Node, error) {
	f.gotCode = code
	return f.lot, f.nodes, f.err
}

func (f *fakeTraceService) Forward(_ context.Context, code string) (genealogy.Lot, []genealogy.Node, error) {
	f.gotCode = code
	return f.lot, f.nodes, f.err
}

func (f *fakeTraceService) Recall(_ context.Context, code string) (genealogy.RecallReport, error) {
	f.gotCode = code
	return f.report, f.err
}

type traceEnv struct {
	handler http.Handler
	tokens  *auth.TokenManager
	service *fakeTraceService
}

func newTraceEnv(t *testing.T) *traceEnv {
	t.Helper()
	tokens := newTestTokens(t)
	service := &fakeTraceService{lot: genealogy.Lot{ID: 1, Code: "RAW-1", Kind: genealogy.KindRaw}}
	mux := http.NewServeMux()
	NewTraceabilityAPI(service).Register(mux, Authenticate(tokens))
	return &traceEnv{handler: mux, tokens: tokens, service: service}
}

func (e *traceEnv) get(t *testing.T, role user.Role, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if role != "" {
		req.Header.Set("Authorization", "Bearer "+issueToken(t, e.tokens, role, auth.AccessToken))
	}
	rec := httptest.NewRecorder()
	e.handler.ServeHTTP(rec, req)
	return rec
}

func TestTraceability_RequiresAuthentication(t *testing.T) {
	for _, path := range []string{
		"/traceability/lots/RAW-1/backward",
		"/traceability/lots/RAW-1/forward",
		"/traceability/lots/RAW-1/recall",
	} {
		if rec := newTraceEnv(t).get(t, "", path); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s: 401 beklenirdi, gelen %d", path, rec.Code)
		}
	}
}

func TestTraceability_RoleMatrix(t *testing.T) {
	cases := map[user.Role]int{
		user.RoleKaliteMuhendisi: http.StatusOK,
		user.RoleUretimPlanlama:  http.StatusOK,
		user.RoleHatSefi:         http.StatusOK,
		user.RoleFabrikaMuduru:   http.StatusOK,
		user.RoleOperator:        http.StatusForbidden,
		user.RoleBakim:           http.StatusForbidden,
	}
	for role, want := range cases {
		t.Run(string(role), func(t *testing.T) {
			if rec := newTraceEnv(t).get(t, role, "/traceability/lots/RAW-1/recall"); rec.Code != want {
				t.Fatalf("%d beklenirdi, gelen %d", want, rec.Code)
			}
		})
	}
}

func TestTraceability_BackwardReturnsLotsWithDepth(t *testing.T) {
	env := newTraceEnv(t)
	env.service.lot = genealogy.Lot{ID: 9, Code: "SN-9", Kind: genealogy.KindSerial}
	env.service.nodes = []genealogy.Node{
		{Lot: genealogy.Lot{ID: 2, Code: "INT-2", Kind: genealogy.KindIntermediate}, Depth: 1},
		{Lot: genealogy.Lot{ID: 1, Code: "RAW-1", Kind: genealogy.KindRaw}, Depth: 2},
	}

	rec := env.get(t, user.RoleKaliteMuhendisi, "/traceability/lots/SN-9/backward")

	if rec.Code != http.StatusOK || env.service.gotCode != "SN-9" {
		t.Fatalf("200 ve kod SN-9 beklenirdi: %d %q", rec.Code, env.service.gotCode)
	}
	var resp traceResponse
	decodeInto(t, rec, &resp)
	if resp.Lot.Code != "SN-9" || len(resp.Lots) != 2 || resp.Lots[1].Kind != "raw" || resp.Lots[1].Depth != 2 {
		t.Errorf("beklenmeyen yanıt: %+v", resp)
	}
}

func TestTraceability_EmptyResultIsArray(t *testing.T) {
	rec := newTraceEnv(t).get(t, user.RoleKaliteMuhendisi, "/traceability/lots/RAW-1/forward")
	if !strings.Contains(rec.Body.String(), `"lots":[]`) {
		t.Errorf("boş sonuç [] olmalı, gövde: %s", rec.Body.String())
	}
}

func TestTraceability_RecallReport(t *testing.T) {
	env := newTraceEnv(t)
	env.service.report = genealogy.RecallReport{
		Lot:             genealogy.Lot{ID: 1, Code: "RAW-1", Kind: genealogy.KindRaw},
		AffectedSerials: []string{"SN-3", "SN-4"},
		Customers:       []string{"A"},
		Shipments: []genealogy.Shipment{
			{Code: "SHP-1", Customer: "A", ShippedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Units: 2},
		},
	}

	rec := env.get(t, user.RoleFabrikaMuduru, "/traceability/lots/RAW-1/recall")

	var resp recallResponse
	decodeInto(t, rec, &resp)
	if resp.AffectedSerialCount != 2 || len(resp.Shipments) != 1 || resp.Shipments[0].Units != 2 || resp.Customers[0] != "A" {
		t.Errorf("beklenmeyen rapor: %+v", resp)
	}
}

func TestTraceability_ErrorMapping(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"bulunamadı", genealogy.ErrLotNotFound, 404, "lot_not_found"},
		{"geçersiz kod", genealogy.ErrInvalidCode, 400, "invalid_code"},
		{"beklenmeyen", errors.New("bağlantı koptu: 10.0.0.13"), 500, "internal_error"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newTraceEnv(t)
			env.service.err = tc.err

			rec := env.get(t, user.RoleKaliteMuhendisi, "/traceability/lots/RAW-1/recall")

			if rec.Code != tc.status {
				t.Fatalf("%d beklenirdi, gelen %d", tc.status, rec.Code)
			}
			if code := decodeError(t, rec).Error.Code; code != tc.code {
				t.Errorf("%s beklenirdi, gelen %q", tc.code, code)
			}
			if strings.Contains(rec.Body.String(), "10.0.0.13") {
				t.Error("iç hata ayrıntısı sızmamalı")
			}
		})
	}
}
