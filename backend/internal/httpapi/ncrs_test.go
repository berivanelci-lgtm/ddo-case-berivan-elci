package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/auth"
	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/ncr"
	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/user"
)

type fakeNCRRepo struct {
	items map[int64]ncr.NCR
	next  int64
	err   error
}

func newFakeNCRRepo() *fakeNCRRepo {
	return &fakeNCRRepo{items: map[int64]ncr.NCR{}, next: 1}
}

func (f *fakeNCRRepo) Create(_ context.Context, n ncr.NCR) (ncr.NCR, error) {
	if f.err != nil {
		return ncr.NCR{}, f.err
	}
	n.ID = f.next
	f.next++
	f.items[n.ID] = n
	return n, nil
}

func (f *fakeNCRRepo) Get(_ context.Context, id int64) (ncr.NCR, error) {
	n, ok := f.items[id]
	if !ok {
		return ncr.NCR{}, ncr.ErrNotFound
	}
	return n, nil
}

func (f *fakeNCRRepo) List(_ context.Context, status ncr.Status, before int64, limit int) ([]ncr.NCR, error) {
	var ids []int64
	for id := range f.items {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] > ids[j] })

	var out []ncr.NCR
	for _, id := range ids {
		n := f.items[id]
		if (before != 0 && id >= before) || (status != "" && n.Status != status) {
			continue
		}
		out = append(out, n)
		if len(out) == limit {
			break
		}
	}
	return out, nil
}

func (f *fakeNCRRepo) Update(_ context.Context, id int64, change func(ncr.NCR) (ncr.NCR, error)) (ncr.NCR, error) {
	n, ok := f.items[id]
	if !ok {
		return ncr.NCR{}, ncr.ErrNotFound
	}
	changed, err := change(n)
	if err != nil {
		return ncr.NCR{}, err
	}
	f.items[id] = changed
	return changed, nil
}

type ncrEnv struct {
	handler http.Handler
	tokens  *auth.TokenManager
	repo    *fakeNCRRepo
}

func newNCREnv(t *testing.T) *ncrEnv {
	t.Helper()
	tokens := newTestTokens(t)
	repo := newFakeNCRRepo()
	mux := http.NewServeMux()
	NewNCRsAPI(repo).Register(mux, Authenticate(tokens))
	return &ncrEnv{handler: mux, tokens: tokens, repo: repo}
}

func (e *ncrEnv) call(t *testing.T, role user.Role, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if role != "" {
		req.Header.Set("Authorization", "Bearer "+issueToken(t, e.tokens, role, auth.AccessToken))
	}
	rec := httptest.NewRecorder()
	e.handler.ServeHTTP(rec, req)
	return rec
}

const validNCRBody = `{"title":"Flanşta çapak","description":"kontrolde bulundu","severity":"major"}`

func (e *ncrEnv) seedOpen(t *testing.T) {
	t.Helper()
	if rec := e.call(t, user.RoleOperator, "POST", "/ncrs", validNCRBody); rec.Code != http.StatusCreated {
		t.Fatalf("ön koşul: 201 beklenirdi, gelen %d", rec.Code)
	}
}

func TestNCRs_RequireAuthentication(t *testing.T) {
	if rec := newNCREnv(t).call(t, "", "POST", "/ncrs", validNCRBody); rec.Code != http.StatusUnauthorized {
		t.Fatalf("401 beklenirdi, gelen %d", rec.Code)
	}
}

func TestNCRs_RoleMatrix(t *testing.T) {
	cases := []struct {
		name string
		role user.Role
		path string
		body string
		want int
	}{
		{"bakım NCR açamaz", user.RoleBakim, "/ncrs", validNCRBody, 403},
		{"planlama NCR açamaz", user.RoleUretimPlanlama, "/ncrs", validNCRBody, 403},
		{"hat şefi NCR açar", user.RoleHatSefi, "/ncrs", validNCRBody, 201},
		{"operatör incelemeye alamaz", user.RoleOperator, "/ncrs/1/investigate", "", 403},
		{"hat şefi kapatamaz", user.RoleHatSefi, "/ncrs/1/close", `{"root_cause":"x","corrective_action":"y"}`, 403},
		{"kalite incelemeye alır", user.RoleKaliteMuhendisi, "/ncrs/1/investigate", "", 200},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newNCREnv(t)
			env.seedOpen(t)

			if rec := env.call(t, tc.role, "POST", tc.path, tc.body); rec.Code != tc.want {
				t.Fatalf("%d beklenirdi, gelen %d (gövde: %s)", tc.want, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestNCRs_FullLifecycle(t *testing.T) {
	env := newNCREnv(t)
	env.seedOpen(t)

	if rec := env.call(t, user.RoleKaliteMuhendisi, "POST", "/ncrs/1/close", `{"root_cause":"x","corrective_action":"y"}`); rec.Code != http.StatusConflict {
		t.Fatalf("açık kayıt doğrudan kapatılamamalı (409), gelen %d", rec.Code)
	}
	if rec := env.call(t, user.RoleKaliteMuhendisi, "POST", "/ncrs/1/investigate", ""); rec.Code != http.StatusOK {
		t.Fatalf("incelemeye alma 200 olmalı, gelen %d", rec.Code)
	}
	if rec := env.call(t, user.RoleKaliteMuhendisi, "POST", "/ncrs/1/close", `{"root_cause":"","corrective_action":"y"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("kök nedensiz kapanış 400 olmalı, gelen %d", rec.Code)
	}

	rec := env.call(t, user.RoleKaliteMuhendisi, "POST", "/ncrs/1/close", `{"root_cause":"takım aşınması","corrective_action":"takım değiştirildi"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("kapanış 200 olmalı, gelen %d", rec.Code)
	}
	var resp ncrResponse
	decodeInto(t, rec, &resp)
	if resp.Status != "closed" || resp.RootCause == nil || *resp.RootCause != "takım aşınması" ||
		resp.ClosedBy == nil || *resp.ClosedBy != 5 || resp.ClosedAt == nil {
		t.Fatalf("beklenmeyen kapanış: %+v", resp)
	}
	if resp.OpenedBy != 5 {
		t.Errorf("açan kullanıcı token'dan gelmeli (5), gelen %d", resp.OpenedBy)
	}
}

func TestNCRs_CreateValidation(t *testing.T) {
	for name, body := range map[string]string{
		"önem yok":        `{"title":"x"}`,
		"bilinmeyen önem": `{"title":"x","severity":"high"}`,
		"başlık yok":      `{"severity":"minor"}`,
		"negatif iş emri": `{"title":"x","severity":"minor","work_order_id":-1}`,
	} {
		t.Run(name, func(t *testing.T) {
			rec := newNCREnv(t).call(t, user.RoleOperator, "POST", "/ncrs", body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("400 beklenirdi, gelen %d", rec.Code)
			}
		})
	}
}

func TestNCRs_NotFoundAndBadID(t *testing.T) {
	env := newNCREnv(t)
	if rec := env.call(t, user.RoleBakim, "GET", "/ncrs/99", ""); rec.Code != http.StatusNotFound {
		t.Errorf("404 beklenirdi, gelen %d", rec.Code)
	}
	if rec := env.call(t, user.RoleBakim, "GET", "/ncrs/abc", ""); rec.Code != http.StatusBadRequest {
		t.Errorf("400 beklenirdi, gelen %d", rec.Code)
	}
}

func TestNCRs_ListFilterAndPaging(t *testing.T) {
	env := newNCREnv(t)
	for i := 0; i < 3; i++ {
		env.seedOpen(t)
	}
	env.call(t, user.RoleKaliteMuhendisi, "POST", "/ncrs/2/investigate", "")

	var page ncrListResponse
	decodeInto(t, env.call(t, user.RoleBakim, "GET", "/ncrs?limit=2", ""), &page)
	if len(page.Items) != 2 || page.Items[0].ID != 3 || page.NextBefore == nil || *page.NextBefore != 2 {
		t.Fatalf("ilk sayfa 3,2 ve next_before 2 olmalı: %+v", page)
	}

	var open ncrListResponse
	decodeInto(t, env.call(t, user.RoleBakim, "GET", "/ncrs?status=open", ""), &open)
	if len(open.Items) != 2 {
		t.Fatalf("2 açık kayıt beklenirdi, gelen %d", len(open.Items))
	}

	if rec := env.call(t, user.RoleBakim, "GET", "/ncrs?status=done", ""); rec.Code != http.StatusBadRequest {
		t.Errorf("geçersiz durum 400 olmalı, gelen %d", rec.Code)
	}
}

func TestNCRs_UnexpectedErrorDoesNotLeak(t *testing.T) {
	env := newNCREnv(t)
	env.repo.err = errors.New("bağlantı koptu: 10.0.0.14")

	rec := env.call(t, user.RoleOperator, "POST", "/ncrs", validNCRBody)
	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "10.0.0.14") {
		t.Fatalf("500 ve sızıntı yok beklenirdi: %d %s", rec.Code, rec.Body.String())
	}
}
