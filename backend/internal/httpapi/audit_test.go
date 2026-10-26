package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/audit"
	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/auth"
	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/user"
)

// --- Sahte okuyucu ---

type fakeAuditReader struct {
	records []audit.Record
	err     error

	gotEntityType string
	gotBefore     int64
	gotLimit      int
}

// List, gerçek okuyucu gibi kimliği before'dan küçük ve türü uyan kayıtları döner.
// Kayıtlar yeniden eskiye sıralı verilmelidir.
func (f *fakeAuditReader) List(_ context.Context, entityType string, before int64, limit int) ([]audit.Record, error) {
	f.gotEntityType, f.gotBefore, f.gotLimit = entityType, before, limit
	if f.err != nil {
		return nil, f.err
	}

	var result []audit.Record
	for _, record := range f.records {
		if before != 0 && record.ID >= before {
			continue
		}
		if entityType != "" && record.EntityType != entityType {
			continue
		}
		result = append(result, record)
		if len(result) == limit {
			break
		}
	}
	return result, nil
}

func auditRecords(from, to int64, entityType string) []audit.Record {
	var records []audit.Record
	for id := from; id >= to; id-- {
		records = append(records, audit.Record{
			ID:         id,
			OccurredAt: time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC),
			Action:     "update",
			EntityType: entityType,
			EntityID:   id,
		})
	}
	return records
}

// --- Test ortamı ---

type auditEnv struct {
	handler http.Handler
	tokens  *auth.TokenManager
	reader  *fakeAuditReader
}

func newAuditEnv(t *testing.T) *auditEnv {
	t.Helper()
	tokens := newTestTokens(t)
	reader := &fakeAuditReader{}

	mux := http.NewServeMux()
	NewAuditAPI(reader).Register(mux, Authenticate(tokens))

	return &auditEnv{handler: mux, tokens: tokens, reader: reader}
}

// get, verilen rolün token'ıyla GET isteği gönderir. Rol boşsa token eklenmez.
func (e *auditEnv) get(t *testing.T, role user.Role, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, strings.NewReader(""))
	if role != "" {
		req.Header.Set("Authorization", "Bearer "+issueToken(t, e.tokens, role, auth.AccessToken))
	}
	rec := httptest.NewRecorder()
	e.handler.ServeHTTP(rec, req)
	return rec
}

// --- Testler ---

func TestAuditLog_RequiresAuthentication(t *testing.T) {
	rec := newAuditEnv(t).get(t, "", "/audit-log")

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("401 beklenirdi, gelen %d", rec.Code)
	}
}

func TestAuditLog_OnlyFactoryManagerCanRead(t *testing.T) {
	cases := map[user.Role]int{
		user.RoleFabrikaMuduru:   http.StatusOK,
		user.RoleOperator:        http.StatusForbidden,
		user.RoleHatSefi:         http.StatusForbidden,
		user.RoleKaliteMuhendisi: http.StatusForbidden,
		user.RoleUretimPlanlama:  http.StatusForbidden,
		user.RoleBakim:           http.StatusForbidden,
	}

	for role, want := range cases {
		t.Run(string(role), func(t *testing.T) {
			rec := newAuditEnv(t).get(t, role, "/audit-log")

			if rec.Code != want {
				t.Fatalf("%d beklenirdi, gelen %d", want, rec.Code)
			}
		})
	}
}

func TestAuditLog_ReturnsOldAndNewValues(t *testing.T) {
	env := newAuditEnv(t)
	actor := int64(7)
	env.reader.records = []audit.Record{{
		ID:         3,
		OccurredAt: time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC),
		UserID:     &actor,
		Action:     "update",
		EntityType: "work_orders",
		EntityID:   12,
		Old:        json.RawMessage(`{"status":"released"}`),
		New:        json.RawMessage(`{"status":"in_progress"}`),
	}}

	rec := env.get(t, user.RoleFabrikaMuduru, "/audit-log")

	body := rec.Body.String()
	for _, want := range []string{
		`"user_id":7`,
		`"action":"update"`,
		`"entity_type":"work_orders"`,
		`"entity_id":12`,
		`"old_value":{"status":"released"}`,
		`"new_value":{"status":"in_progress"}`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("yanıtta %s bulunmalı, gövde: %s", want, body)
		}
	}
}

func TestAuditLog_NullUserAndValuesAreNull(t *testing.T) {
	env := newAuditEnv(t)
	env.reader.records = auditRecords(1, 1, "charts")

	rec := env.get(t, user.RoleFabrikaMuduru, "/audit-log")

	body := rec.Body.String()
	for _, want := range []string{`"user_id":null`, `"old_value":null`, `"new_value":null`} {
		if !strings.Contains(body, want) {
			t.Errorf("yanıtta %s bulunmalı, gövde: %s", want, body)
		}
	}
}

func TestAuditLog_EmptyListIsArray(t *testing.T) {
	rec := newAuditEnv(t).get(t, user.RoleFabrikaMuduru, "/audit-log")

	for _, want := range []string{`"items":[]`, `"next_before":null`} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Errorf("yanıtta %s bulunmalı, gövde: %s", want, rec.Body.String())
		}
	}
}

func TestAuditLog_KeysetPaging(t *testing.T) {
	env := newAuditEnv(t)
	env.reader.records = auditRecords(5, 1, "materials")

	rec := env.get(t, user.RoleFabrikaMuduru, "/audit-log?limit=2")
	var first auditListResponse
	decodeInto(t, rec, &first)
	if len(first.Items) != 2 || first.Items[0].ID != 5 || first.Items[1].ID != 4 {
		t.Fatalf("5 ve 4 beklenirdi: %+v", first.Items)
	}
	if first.NextBefore == nil || *first.NextBefore != 4 {
		t.Fatalf("next_before 4 olmalı: %v", first.NextBefore)
	}
	if env.reader.gotLimit != 3 {
		t.Errorf("okuyucudan limit+1 (3) istenmeli, gelen %d", env.reader.gotLimit)
	}

	rec = env.get(t, user.RoleFabrikaMuduru, "/audit-log?limit=2&before=4")
	var second auditListResponse
	decodeInto(t, rec, &second)
	if len(second.Items) != 2 || second.Items[0].ID != 3 || second.Items[1].ID != 2 {
		t.Fatalf("3 ve 2 beklenirdi: %+v", second.Items)
	}

	rec = env.get(t, user.RoleFabrikaMuduru, "/audit-log?limit=2&before=2")
	var last auditListResponse
	decodeInto(t, rec, &last)
	if len(last.Items) != 1 || last.Items[0].ID != 1 || last.NextBefore != nil {
		t.Fatalf("son sayfada yalnızca 1 kayıt ve next_before null beklenirdi: %+v", last)
	}
}

func TestAuditLog_FiltersByEntityType(t *testing.T) {
	env := newAuditEnv(t)
	env.reader.records = append(auditRecords(4, 3, "materials"), auditRecords(2, 1, "charts")...)

	rec := env.get(t, user.RoleFabrikaMuduru, "/audit-log?entity_type=materials")

	var resp auditListResponse
	decodeInto(t, rec, &resp)
	if env.reader.gotEntityType != "materials" {
		t.Errorf("okuyucuya materials gitmeli, gelen %q", env.reader.gotEntityType)
	}
	if len(resp.Items) != 2 {
		t.Fatalf("2 kayıt beklenirdi, gelen %d", len(resp.Items))
	}
	for _, item := range resp.Items {
		if item.EntityType != "materials" {
			t.Errorf("yalnızca materials beklenirdi, gelen %q", item.EntityType)
		}
	}
}

func TestAuditLog_RejectsUnknownEntityType(t *testing.T) {
	env := newAuditEnv(t)

	rec := env.get(t, user.RoleFabrikaMuduru, "/audit-log?entity_type=users")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("400 beklenirdi, gelen %d", rec.Code)
	}
	if code := decodeError(t, rec).Error.Code; code != "invalid_entity_type" {
		t.Errorf("invalid_entity_type beklenirdi, gelen %q", code)
	}
	if env.reader.gotLimit != 0 {
		t.Error("geçersiz istekte okuyucu çağrılmamalı")
	}
}

func TestAuditLog_RejectsInvalidPaging(t *testing.T) {
	for _, query := range []string{"limit=0", "limit=101", "before=0", "before=abc"} {
		t.Run(query, func(t *testing.T) {
			rec := newAuditEnv(t).get(t, user.RoleFabrikaMuduru, "/audit-log?"+query)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("400 beklenirdi, gelen %d", rec.Code)
			}
		})
	}
}

func TestAuditLog_ReaderErrorDoesNotLeakDetails(t *testing.T) {
	env := newAuditEnv(t)
	env.reader.err = errors.New("bağlantı koptu: 10.0.0.11")

	rec := env.get(t, user.RoleFabrikaMuduru, "/audit-log")

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("500 beklenirdi, gelen %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "10.0.0.11") {
		t.Error("iç hata ayrıntısı kullanıcıya sızmamalı")
	}
}

// Denetim kaydında "kim" bilgisi token'dan gelir: Authenticate kullanıcıyı denetim context'ine koymalı.
func TestAuthenticate_PutsUserIntoAuditContext(t *testing.T) {
	tokens := newTestTokens(t)
	var (
		gotUser int64
		gotOK   bool
	)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser, gotOK = audit.UserFrom(r.Context())
		w.WriteHeader(http.StatusOK)
	})
	handler := Authenticate(tokens)(next)

	rec := getWithAuth(handler, "Bearer "+issueToken(t, tokens, user.RoleOperator, auth.AccessToken))

	if rec.Code != http.StatusOK {
		t.Fatalf("200 beklenirdi, gelen %d", rec.Code)
	}
	if !gotOK || gotUser != 5 {
		t.Fatalf("denetim context'inde kullanıcı 5 olmalı, gelen %d (bulundu: %v)", gotUser, gotOK)
	}
}
