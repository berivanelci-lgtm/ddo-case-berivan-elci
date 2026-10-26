package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/auth"
	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/sensor"
	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/user"
)

type fakeIngester struct {
	got []sensor.Reading
	err error
}

func (f *fakeIngester) Ingest(_ context.Context, readings []sensor.Reading) (int64, error) {
	f.got = readings
	if f.err != nil {
		return 0, f.err
	}
	return int64(len(readings)), nil
}

const sensorBody = `{"readings":[{"station_id":3,"recorded_at":"2026-10-26T17:00:00Z","value":50.5},{"station_id":4,"recorded_at":"2026-10-26T17:00:01Z","value":49.5}]}`

func sensorCall(t *testing.T, ingester *fakeIngester, role user.Role, body string) *httptest.ResponseRecorder {
	t.Helper()
	tokens := newTestTokens(t)
	mux := http.NewServeMux()
	NewSensorsAPI(ingester).Register(mux, Authenticate(tokens))

	req := httptest.NewRequest(http.MethodPost, "/sensor-readings", strings.NewReader(body))
	if role != "" {
		req.Header.Set("Authorization", "Bearer "+issueToken(t, tokens, role, auth.AccessToken))
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestSensors_RoleMatrix(t *testing.T) {
	cases := map[user.Role]int{
		"":                       401,
		user.RoleOperator:        201,
		user.RoleHatSefi:         201,
		user.RoleBakim:           201,
		user.RoleKaliteMuhendisi: 403,
		user.RoleFabrikaMuduru:   403,
	}
	for role, want := range cases {
		if rec := sensorCall(t, &fakeIngester{}, role, sensorBody); rec.Code != want {
			t.Errorf("%q: %d beklenirdi, gelen %d", role, want, rec.Code)
		}
	}
}

func TestSensors_PassesReadingsAndReturnsCount(t *testing.T) {
	ingester := &fakeIngester{}
	rec := sensorCall(t, ingester, user.RoleBakim, sensorBody)

	var resp ingestResponse
	decodeInto(t, rec, &resp)
	if resp.Inserted != 2 || len(ingester.got) != 2 || ingester.got[0].StationID != 3 || ingester.got[1].Value != 49.5 {
		t.Fatalf("beklenmeyen sonuç: %+v %+v", resp, ingester.got)
	}
}

func TestSensors_ErrorMapping(t *testing.T) {
	if rec := sensorCall(t, &fakeIngester{err: sensor.ErrInvalid}, user.RoleBakim, sensorBody); rec.Code != 400 {
		t.Errorf("geçersiz veri 400 olmalı, gelen %d", rec.Code)
	}
	rec := sensorCall(t, &fakeIngester{err: errors.New("disk dolu: 10.0.0.16")}, user.RoleBakim, sensorBody)
	if rec.Code != 500 || strings.Contains(rec.Body.String(), "10.0.0.16") {
		t.Errorf("500 ve sızıntı yok beklenirdi: %d %s", rec.Code, rec.Body.String())
	}
	if rec := sensorCall(t, &fakeIngester{}, user.RoleBakim, `{"readings":"x"}`); rec.Code != 400 {
		t.Errorf("bozuk gövde 400 olmalı, gelen %d", rec.Code)
	}
}
