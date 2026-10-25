package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/auth"
	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/user"
	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/workorder"
)

// --- Sahte servis ve depo ---

type fakeOrderService struct {
	order workorder.WorkOrder
	err   error

	calls      []string
	gotID      int64
	gotGood    int64
	gotScrap   int64
	gotRework  int64
	gotUserID  int64
	gotPartial bool
}

func (f *fakeOrderService) Release(_ context.Context, id int64) (workorder.WorkOrder, error) {
	f.calls, f.gotID = append(f.calls, "release"), id
	return f.order, f.err
}

func (f *fakeOrderService) Cancel(_ context.Context, id int64) (workorder.WorkOrder, error) {
	f.calls, f.gotID = append(f.calls, "cancel"), id
	return f.order, f.err
}

func (f *fakeOrderService) RecordProduction(_ context.Context, id, good, scrap, rework, userID int64) (workorder.WorkOrder, error) {
	f.calls, f.gotID = append(f.calls, "production"), id
	f.gotGood, f.gotScrap, f.gotRework, f.gotUserID = good, scrap, rework, userID
	return f.order, f.err
}

func (f *fakeOrderService) RecordRework(_ context.Context, id, good, scrap, userID int64) (workorder.WorkOrder, error) {
	f.calls, f.gotID = append(f.calls, "rework"), id
	f.gotGood, f.gotScrap, f.gotUserID = good, scrap, userID
	return f.order, f.err
}

func (f *fakeOrderService) Close(_ context.Context, id int64, allowPartial bool) (workorder.WorkOrder, error) {
	f.calls, f.gotID, f.gotPartial = append(f.calls, "close"), id, allowPartial
	return f.order, f.err
}

type fakeOrderRepo struct {
	orders []workorder.WorkOrder
	err    error

	gotBefore int64
	gotLimit  int
	createdBy int64
}

func (f *fakeOrderRepo) Create(_ context.Context, productID, plannedQty, createdBy int64) (workorder.WorkOrder, error) {
	if f.err != nil {
		return workorder.WorkOrder{}, f.err
	}
	order, err := workorder.New(productID, plannedQty)
	if err != nil {
		return workorder.WorkOrder{}, err
	}
	f.createdBy = createdBy
	order.ID = 12
	return order, nil
}

func (f *fakeOrderRepo) Get(_ context.Context, id int64) (workorder.WorkOrder, error) {
	for _, order := range f.orders {
		if order.ID == id {
			return order, nil
		}
	}
	return workorder.WorkOrder{}, workorder.ErrOrderNotFound
}

// List, gerçek depo gibi kimliği before'dan küçük kayıtları yeniden eskiye sıralı döner.
func (f *fakeOrderRepo) List(_ context.Context, before int64, limit int) ([]workorder.WorkOrder, error) {
	f.gotBefore, f.gotLimit = before, limit
	if f.err != nil {
		return nil, f.err
	}

	var result []workorder.WorkOrder
	for _, order := range f.orders {
		if before != 0 && order.ID >= before {
			continue
		}
		result = append(result, order)
		if len(result) == limit {
			break
		}
	}
	return result, nil
}

// --- Test ortamı ---

type ordersEnv struct {
	handler http.Handler
	tokens  *auth.TokenManager
	service *fakeOrderService
	repo    *fakeOrderRepo
}

func newOrdersEnv(t *testing.T) *ordersEnv {
	t.Helper()
	tokens := newTestTokens(t)
	service := &fakeOrderService{order: workorder.WorkOrder{ID: 1, ProductID: 7, PlannedQty: 10, Status: workorder.StatusReleased}}
	repo := &fakeOrderRepo{}

	mux := http.NewServeMux()
	NewWorkOrdersAPI(service, repo).Register(mux, Authenticate(tokens))

	return &ordersEnv{handler: mux, tokens: tokens, service: service, repo: repo}
}

func (e *ordersEnv) call(t *testing.T, role user.Role, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if role != "" {
		req.Header.Set("Authorization", "Bearer "+issueToken(t, e.tokens, role, auth.AccessToken))
	}
	rec := httptest.NewRecorder()
	e.handler.ServeHTTP(rec, req)
	return rec
}

// --- Kimlik ve yetki ---

func TestWorkOrders_RequireAuthentication(t *testing.T) {
	endpoints := []struct{ name, method, path string }{
		{"olustur", http.MethodPost, "/work-orders"},
		{"listele", http.MethodGet, "/work-orders"},
		{"detay", http.MethodGet, "/work-orders/1"},
		{"serbest_birak", http.MethodPost, "/work-orders/1/release"},
		{"iptal", http.MethodPost, "/work-orders/1/cancel"},
		{"uretim", http.MethodPost, "/work-orders/1/production"},
		{"yeniden_islem", http.MethodPost, "/work-orders/1/rework"},
		{"kapat", http.MethodPost, "/work-orders/1/close"},
	}

	for _, endpoint := range endpoints {
		t.Run(endpoint.name, func(t *testing.T) {
			rec := newOrdersEnv(t).call(t, "", endpoint.method, endpoint.path, "")

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("401 beklenirdi, gelen %d", rec.Code)
			}
		})
	}
}

func TestWorkOrders_RoleMatrix(t *testing.T) {
	const (
		createBody = `{"product_id":7,"planned_qty":10}`
		prodBody   = `{"good":1}`
		reworkBody = `{"good":1}`
	)

	cases := []struct {
		name         string
		role         user.Role
		method, path string
		body         string
		want         int
	}{
		{"planlama iş emri açar", user.RoleUretimPlanlama, "POST", "/work-orders", createBody, 201},
		{"müdür iş emri açar", user.RoleFabrikaMuduru, "POST", "/work-orders", createBody, 201},
		{"operatör iş emri açamaz", user.RoleOperator, "POST", "/work-orders", createBody, 403},
		{"hat şefi iş emri açamaz", user.RoleHatSefi, "POST", "/work-orders", createBody, 403},

		{"planlama serbest bırakır", user.RoleUretimPlanlama, "POST", "/work-orders/1/release", "", 200},
		{"hat şefi serbest bırakır", user.RoleHatSefi, "POST", "/work-orders/1/release", "", 200},
		{"operatör serbest bırakamaz", user.RoleOperator, "POST", "/work-orders/1/release", "", 403},
		{"bakım serbest bırakamaz", user.RoleBakim, "POST", "/work-orders/1/release", "", 403},

		{"planlama iptal eder", user.RoleUretimPlanlama, "POST", "/work-orders/1/cancel", "", 200},
		{"hat şefi iptal edemez", user.RoleHatSefi, "POST", "/work-orders/1/cancel", "", 403},

		{"operatör üretim girer", user.RoleOperator, "POST", "/work-orders/1/production", prodBody, 200},
		{"hat şefi üretim girer", user.RoleHatSefi, "POST", "/work-orders/1/production", prodBody, 200},
		{"kalite üretim giremez", user.RoleKaliteMuhendisi, "POST", "/work-orders/1/production", prodBody, 403},
		{"planlama üretim giremez", user.RoleUretimPlanlama, "POST", "/work-orders/1/production", prodBody, 403},

		{"kalite yeniden işlem girer", user.RoleKaliteMuhendisi, "POST", "/work-orders/1/rework", reworkBody, 200},
		{"operatör yeniden işlem girer", user.RoleOperator, "POST", "/work-orders/1/rework", reworkBody, 200},
		{"bakım yeniden işlem giremez", user.RoleBakim, "POST", "/work-orders/1/rework", reworkBody, 403},

		{"hat şefi kapatır", user.RoleHatSefi, "POST", "/work-orders/1/close", "", 200},
		{"müdür kapatır", user.RoleFabrikaMuduru, "POST", "/work-orders/1/close", "", 200},
		{"operatör kapatamaz", user.RoleOperator, "POST", "/work-orders/1/close", "", 403},
		{"kalite kapatamaz", user.RoleKaliteMuhendisi, "POST", "/work-orders/1/close", "", 403},

		{"bakım iş emirlerini listeler", user.RoleBakim, "GET", "/work-orders", "", 200},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newOrdersEnv(t)
			env.repo.orders = []workorder.WorkOrder{{ID: 1}}

			rec := env.call(t, tc.role, tc.method, tc.path, tc.body)

			if rec.Code != tc.want {
				t.Fatalf("%d beklenirdi, gelen %d (gövde: %s)", tc.want, rec.Code, rec.Body.String())
			}
			if tc.want == http.StatusForbidden && len(env.service.calls) != 0 {
				t.Errorf("yetkisiz istekte servis çağrılmamalı, çağrılan: %v", env.service.calls)
			}
		})
	}
}

// --- Oluşturma ---

func TestCreateOrder_Success(t *testing.T) {
	env := newOrdersEnv(t)

	rec := env.call(t, user.RoleUretimPlanlama, "POST", "/work-orders", `{"product_id":7,"planned_qty":10}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("201 beklenirdi, gelen %d", rec.Code)
	}
	var resp orderResponse
	decodeInto(t, rec, &resp)
	if resp.ID != 12 || resp.ProductID != 7 || resp.PlannedQty != 10 || resp.Status != "planned" {
		t.Errorf("beklenmeyen yanıt: %+v", resp)
	}
	if env.repo.createdBy != 5 {
		t.Errorf("oluşturan token'daki kullanıcı (5) olmalı, gelen %d", env.repo.createdBy)
	}
}

func TestCreateOrder_Validation(t *testing.T) {
	cases := []struct {
		name   string
		body   string
		status int
		code   string
	}{
		{"ürün yok", `{"planned_qty":10}`, 400, "validation_error"},
		{"negatif ürün", `{"product_id":-1,"planned_qty":10}`, 400, "validation_error"},
		{"sıfır miktar", `{"product_id":7,"planned_qty":0}`, 400, "invalid_quantity"},
		{"negatif miktar", `{"product_id":7,"planned_qty":-3}`, 400, "invalid_quantity"},
		{"bilinmeyen alan", `{"product_id":7,"planned_qty":10,"status":"completed"}`, 400, "invalid_request"},
		{"bozuk json", `{bozuk`, 400, "invalid_request"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := newOrdersEnv(t).call(t, user.RoleUretimPlanlama, "POST", "/work-orders", tc.body)

			if rec.Code != tc.status {
				t.Fatalf("%d beklenirdi, gelen %d", tc.status, rec.Code)
			}
			if code := decodeError(t, rec).Error.Code; code != tc.code {
				t.Errorf("%s beklenirdi, gelen %q", tc.code, code)
			}
		})
	}
}

func TestCreateOrder_UnknownProduct(t *testing.T) {
	env := newOrdersEnv(t)
	env.repo.err = workorder.ErrProductNotFound

	rec := env.call(t, user.RoleUretimPlanlama, "POST", "/work-orders", `{"product_id":99,"planned_qty":10}`)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("422 beklenirdi, gelen %d", rec.Code)
	}
	if code := decodeError(t, rec).Error.Code; code != "product_not_found" {
		t.Errorf("product_not_found beklenirdi, gelen %q", code)
	}
}

// --- Üretim, yeniden işlem ve kapanış ---

func TestProduction_PassesValuesAndUserToService(t *testing.T) {
	env := newOrdersEnv(t)
	env.service.order = workorder.WorkOrder{
		ID: 1, PlannedQty: 10, Status: workorder.StatusInProgress,
		StartedQty: 8, GoodQty: 5, ScrapQty: 2, ReworkPending: 1,
	}

	rec := env.call(t, user.RoleOperator, "POST", "/work-orders/1/production", `{"good":5,"scrap":2,"rework":1}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("200 beklenirdi, gelen %d", rec.Code)
	}
	if env.service.gotID != 1 || env.service.gotGood != 5 || env.service.gotScrap != 2 ||
		env.service.gotRework != 1 || env.service.gotUserID != 5 {
		t.Errorf("servise yanlış değerler gitti: %+v", env.service)
	}
	var resp orderResponse
	decodeInto(t, rec, &resp)
	if resp.Status != "in_progress" || resp.StartedQty != 8 || resp.ReworkPending != 1 {
		t.Errorf("beklenmeyen yanıt: %+v", resp)
	}
}

func TestRework_PassesValuesAndUserToService(t *testing.T) {
	env := newOrdersEnv(t)

	rec := env.call(t, user.RoleKaliteMuhendisi, "POST", "/work-orders/1/rework", `{"good":3,"scrap":1}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("200 beklenirdi, gelen %d", rec.Code)
	}
	if env.service.gotGood != 3 || env.service.gotScrap != 1 || env.service.gotUserID != 5 {
		t.Errorf("servise yanlış değerler gitti: %+v", env.service)
	}
}

func TestClose_AllowPartialFlag(t *testing.T) {
	cases := []struct {
		name string
		body string
		want bool
	}{
		{"gövde yok", "", false},
		{"onay yok", `{}`, false},
		{"onaylı", `{"allow_partial":true}`, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newOrdersEnv(t)

			rec := env.call(t, user.RoleHatSefi, "POST", "/work-orders/1/close", tc.body)

			if rec.Code != http.StatusOK {
				t.Fatalf("200 beklenirdi, gelen %d", rec.Code)
			}
			if env.service.gotPartial != tc.want {
				t.Errorf("allow_partial %v olmalı, gelen %v", tc.want, env.service.gotPartial)
			}
		})
	}
}

func TestClose_RejectsUnknownFields(t *testing.T) {
	rec := newOrdersEnv(t).call(t, user.RoleHatSefi, "POST", "/work-orders/1/close", `{"status":"completed"}`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("400 beklenirdi, gelen %d", rec.Code)
	}
}

func TestOrders_ServiceErrorMapping(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"iş emri yok", workorder.ErrOrderNotFound, 404, "order_not_found"},
		{"geçersiz geçiş", fmt.Errorf("%w: ayrıntı", workorder.ErrInvalidTransition), 409, "invalid_transition"},
		{"planı aşıyor", workorder.ErrExceedsPlan, 409, "exceeds_plan"},
		{"stok yetersiz", fmt.Errorf("%w: malzeme 2", workorder.ErrInsufficientStock), 409, "insufficient_stock"},
		{"yeniden işlem bekliyor", workorder.ErrReworkPending, 409, "rework_pending"},
		{"kısmi onay yok", workorder.ErrPartialNotAllowed, 409, "partial_not_confirmed"},
		{"geçersiz miktar", workorder.ErrInvalidQuantity, 400, "invalid_quantity"},
		{"boş reçete", workorder.ErrEmptyBOM, 422, "invalid_bom"},
		{"beklenmeyen hata", errors.New("bağlantı koptu: 10.0.0.8"), 500, "internal_error"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newOrdersEnv(t)
			env.service.err = tc.err

			rec := env.call(t, user.RoleOperator, "POST", "/work-orders/1/production", `{"good":1}`)

			if rec.Code != tc.status {
				t.Fatalf("%d beklenirdi, gelen %d", tc.status, rec.Code)
			}
			if code := decodeError(t, rec).Error.Code; code != tc.code {
				t.Errorf("%s beklenirdi, gelen %q", tc.code, code)
			}
			if tc.status == http.StatusInternalServerError && strings.Contains(rec.Body.String(), "10.0.0.8") {
				t.Error("iç hata ayrıntısı kullanıcıya sızmamalı")
			}
		})
	}
}

func TestOrders_InvalidIDNeverReachesService(t *testing.T) {
	for _, path := range []string{
		"/work-orders/abc/production",
		"/work-orders/0/production",
		"/work-orders/-4/production",
	} {
		t.Run(path, func(t *testing.T) {
			env := newOrdersEnv(t)

			rec := env.call(t, user.RoleOperator, "POST", path, `{"good":1}`)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("400 beklenirdi, gelen %d", rec.Code)
			}
			if code := decodeError(t, rec).Error.Code; code != "invalid_id" {
				t.Errorf("invalid_id beklenirdi, gelen %q", code)
			}
			if len(env.service.calls) != 0 {
				t.Error("geçersiz kimlikle servis çağrılmamalı")
			}
		})
	}
}

// --- Okuma ve sayfalama ---

func TestGetOrder(t *testing.T) {
	env := newOrdersEnv(t)
	env.repo.orders = []workorder.WorkOrder{{ID: 1, ProductID: 7, PlannedQty: 10, Status: workorder.StatusReleased}}

	rec := env.call(t, user.RoleBakim, "GET", "/work-orders/1", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("200 beklenirdi, gelen %d", rec.Code)
	}
	var resp orderResponse
	decodeInto(t, rec, &resp)
	if resp.ID != 1 || resp.Status != "released" {
		t.Errorf("beklenmeyen yanıt: %+v", resp)
	}

	rec = env.call(t, user.RoleBakim, "GET", "/work-orders/99", "")
	if rec.Code != http.StatusNotFound {
		t.Errorf("olmayan iş emri için 404 beklenirdi, gelen %d", rec.Code)
	}
}

func orderRange(from, to int64) []workorder.WorkOrder {
	var orders []workorder.WorkOrder
	for id := from; id >= to; id-- {
		orders = append(orders, workorder.WorkOrder{ID: id, Status: workorder.StatusPlanned})
	}
	return orders
}

func TestListOrders_KeysetPaging(t *testing.T) {
	env := newOrdersEnv(t)
	env.repo.orders = orderRange(5, 1)

	// 1. sayfa: 2 kayıt, devamı var.
	rec := env.call(t, user.RoleBakim, "GET", "/work-orders?limit=2", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("200 beklenirdi, gelen %d", rec.Code)
	}
	var first orderListResponse
	decodeInto(t, rec, &first)
	if len(first.Items) != 2 || first.Items[0].ID != 5 || first.Items[1].ID != 4 {
		t.Fatalf("5 ve 4 beklenirdi: %+v", first.Items)
	}
	if first.NextBefore == nil || *first.NextBefore != 4 {
		t.Fatalf("next_before 4 olmalı: %v", first.NextBefore)
	}
	if env.repo.gotLimit != 3 {
		t.Errorf("depodan limit+1 (3) istenmeli, gelen %d", env.repo.gotLimit)
	}

	// 2. sayfa
	rec = env.call(t, user.RoleBakim, "GET", "/work-orders?limit=2&before=4", "")
	var second orderListResponse
	decodeInto(t, rec, &second)
	if len(second.Items) != 2 || second.Items[0].ID != 3 || second.Items[1].ID != 2 {
		t.Fatalf("3 ve 2 beklenirdi: %+v", second.Items)
	}
	if second.NextBefore == nil || *second.NextBefore != 2 {
		t.Fatalf("next_before 2 olmalı: %v", second.NextBefore)
	}

	// Son sayfa: tek kayıt, devamı yok.
	rec = env.call(t, user.RoleBakim, "GET", "/work-orders?limit=2&before=2", "")
	var last orderListResponse
	decodeInto(t, rec, &last)
	if len(last.Items) != 1 || last.Items[0].ID != 1 {
		t.Fatalf("yalnızca 1 beklenirdi: %+v", last.Items)
	}
	if last.NextBefore != nil {
		t.Errorf("son sayfada next_before null olmalı, gelen %d", *last.NextBefore)
	}
}

func TestListOrders_ExactPageSizeHasNoNextPage(t *testing.T) {
	env := newOrdersEnv(t)
	env.repo.orders = orderRange(2, 1)

	rec := env.call(t, user.RoleBakim, "GET", "/work-orders?limit=2", "")

	var resp orderListResponse
	decodeInto(t, rec, &resp)
	if len(resp.Items) != 2 || resp.NextBefore != nil {
		t.Errorf("tam sayfa boyutunda next_before null olmalı: %+v", resp)
	}
}

func TestListOrders_DefaultLimitAndEmptyList(t *testing.T) {
	env := newOrdersEnv(t)

	rec := env.call(t, user.RoleBakim, "GET", "/work-orders", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("200 beklenirdi, gelen %d", rec.Code)
	}
	if env.repo.gotLimit != defaultOrderPageSize+1 {
		t.Errorf("varsayılan limit %d olmalı, depoya %d gitti", defaultOrderPageSize, env.repo.gotLimit)
	}
	for _, want := range []string{`"items":[]`, `"next_before":null`} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Errorf("yanıtta %s bulunmalı, gövde: %s", want, rec.Body.String())
		}
	}
}

func TestListOrders_RejectsInvalidPaging(t *testing.T) {
	cases := []struct{ name, query, code string }{
		{"limit sıfır", "limit=0", "invalid_limit"},
		{"limit çok büyük", "limit=101", "invalid_limit"},
		{"limit sayı değil", "limit=abc", "invalid_limit"},
		{"before sıfır", "before=0", "invalid_cursor"},
		{"before negatif", "before=-5", "invalid_cursor"},
		{"before sayı değil", "before=abc", "invalid_cursor"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := newOrdersEnv(t).call(t, user.RoleBakim, "GET", "/work-orders?"+tc.query, "")

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("400 beklenirdi, gelen %d", rec.Code)
			}
			if code := decodeError(t, rec).Error.Code; code != tc.code {
				t.Errorf("%s beklenirdi, gelen %q", tc.code, code)
			}
		})
	}
}
