package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/auth"
	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/catalog"
	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/user"
)

const (
	validMaterialBody = `{"code":"CLK-01","name":"Çelik plaka","stock_qty":100}`
	validProductBody  = `{"code":"P-1","name":"Fren diski","bom":[{"material_id":1,"qty_per_unit":5}]}`
)

// --- Sahte depo ---

type fakeCatalogRepo struct {
	materials []catalog.Material
	products  []catalog.Product
	err       error

	createdMaterialCode string
	createdProductCode  string
}

func (f *fakeCatalogRepo) CreateMaterial(_ context.Context, code, name string, stockQty int64) (catalog.Material, error) {
	if f.err != nil {
		return catalog.Material{}, f.err
	}
	f.createdMaterialCode = code
	return catalog.Material{ID: 3, Code: code, Name: name, StockQty: stockQty}, nil
}

func (f *fakeCatalogRepo) ListMaterials(context.Context) ([]catalog.Material, error) {
	return f.materials, f.err
}

func (f *fakeCatalogRepo) CreateProduct(_ context.Context, code, name string, bom []catalog.BOMItem) (catalog.Product, error) {
	if f.err != nil {
		return catalog.Product{}, f.err
	}
	f.createdProductCode = code
	return catalog.Product{ID: 4, Code: code, Name: name, BOM: bom}, nil
}

func (f *fakeCatalogRepo) ListProducts(context.Context) ([]catalog.Product, error) {
	return f.products, f.err
}

// --- Test ortamı ---

type catalogEnv struct {
	handler http.Handler
	tokens  *auth.TokenManager
	repo    *fakeCatalogRepo
}

func newCatalogEnv(t *testing.T) *catalogEnv {
	t.Helper()
	tokens := newTestTokens(t)
	repo := &fakeCatalogRepo{}

	mux := http.NewServeMux()
	NewCatalogAPI(repo).Register(mux, Authenticate(tokens))

	return &catalogEnv{handler: mux, tokens: tokens, repo: repo}
}

// call, verilen rolün token'ıyla bir istek gönderir. Rol boşsa token eklenmez.
func (e *catalogEnv) call(t *testing.T, role user.Role, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if role != "" {
		req.Header.Set("Authorization", "Bearer "+issueToken(t, e.tokens, role, auth.AccessToken))
	}
	rec := httptest.NewRecorder()
	e.handler.ServeHTTP(rec, req)
	return rec
}

// --- Testler ---

func TestCatalog_RequireAuthentication(t *testing.T) {
	endpoints := []struct{ name, method, path string }{
		{"malzeme_olustur", http.MethodPost, "/materials"},
		{"malzeme_listele", http.MethodGet, "/materials"},
		{"urun_olustur", http.MethodPost, "/products"},
		{"urun_listele", http.MethodGet, "/products"},
	}

	for _, endpoint := range endpoints {
		t.Run(endpoint.name, func(t *testing.T) {
			rec := newCatalogEnv(t).call(t, "", endpoint.method, endpoint.path, "")

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("401 beklenirdi, gelen %d", rec.Code)
			}
		})
	}
}

func TestCatalog_RoleMatrix(t *testing.T) {
	cases := []struct {
		name         string
		role         user.Role
		method, path string
		body         string
		want         int
	}{
		{"planlama malzeme oluşturur", user.RoleUretimPlanlama, "POST", "/materials", validMaterialBody, 201},
		{"müdür malzeme oluşturur", user.RoleFabrikaMuduru, "POST", "/materials", validMaterialBody, 201},
		{"operatör malzeme oluşturamaz", user.RoleOperator, "POST", "/materials", validMaterialBody, 403},
		{"hat şefi malzeme oluşturamaz", user.RoleHatSefi, "POST", "/materials", validMaterialBody, 403},
		{"bakım malzeme oluşturamaz", user.RoleBakim, "POST", "/materials", validMaterialBody, 403},

		{"planlama ürün oluşturur", user.RoleUretimPlanlama, "POST", "/products", validProductBody, 201},
		{"müdür ürün oluşturur", user.RoleFabrikaMuduru, "POST", "/products", validProductBody, 201},
		{"hat şefi ürün oluşturamaz", user.RoleHatSefi, "POST", "/products", validProductBody, 403},
		{"kalite ürün oluşturamaz", user.RoleKaliteMuhendisi, "POST", "/products", validProductBody, 403},

		{"operatör malzemeleri listeler", user.RoleOperator, "GET", "/materials", "", 200},
		{"bakım ürünleri listeler", user.RoleBakim, "GET", "/products", "", 200},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := newCatalogEnv(t).call(t, tc.role, tc.method, tc.path, tc.body)

			if rec.Code != tc.want {
				t.Fatalf("%d beklenirdi, gelen %d (gövde: %s)", tc.want, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestCreateMaterial_Success(t *testing.T) {
	env := newCatalogEnv(t)

	rec := env.call(t, user.RoleUretimPlanlama, "POST", "/materials",
		`{"code":"  CLK-01 ","name":"Çelik plaka","stock_qty":100}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("201 beklenirdi, gelen %d", rec.Code)
	}
	var resp materialResponse
	decodeInto(t, rec, &resp)
	if resp.ID != 3 || resp.Code != "CLK-01" || resp.Name != "Çelik plaka" || resp.StockQty != 100 {
		t.Errorf("beklenmeyen yanıt: %+v", resp)
	}
	if env.repo.createdMaterialCode != "CLK-01" {
		t.Errorf("depoya temizlenmiş kod gitmeli, gelen %q", env.repo.createdMaterialCode)
	}
}

func TestCreateMaterial_Validation(t *testing.T) {
	cases := map[string]string{
		"boş kod":      `{"code":"","name":"Ad","stock_qty":1}`,
		"boş ad":       `{"code":"K","name":" ","stock_qty":1}`,
		"negatif stok": `{"code":"K","name":"Ad","stock_qty":-1}`,
		"sınırı aşan":  `{"code":"K","name":"Ad","stock_qty":1000000001}`,
		"uzun kod":     `{"code":"` + strings.Repeat("a", 41) + `","name":"Ad","stock_qty":1}`,
	}

	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			env := newCatalogEnv(t)

			rec := env.call(t, user.RoleUretimPlanlama, "POST", "/materials", body)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("400 beklenirdi, gelen %d", rec.Code)
			}
			if code := decodeError(t, rec).Error.Code; code != "validation_error" {
				t.Errorf("validation_error beklenirdi, gelen %q", code)
			}
			if env.repo.createdMaterialCode != "" {
				t.Error("geçersiz istekle malzeme oluşturulmamalı")
			}
		})
	}
}

func TestCreateMaterial_RejectsUnknownFields(t *testing.T) {
	rec := newCatalogEnv(t).call(t, user.RoleUretimPlanlama, "POST", "/materials",
		`{"code":"K","name":"Ad","stock_qty":1,"id":99}`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("400 beklenirdi, gelen %d", rec.Code)
	}
	if code := decodeError(t, rec).Error.Code; code != "invalid_request" {
		t.Errorf("invalid_request beklenirdi, gelen %q", code)
	}
}

func TestCreateMaterial_DuplicateCode(t *testing.T) {
	env := newCatalogEnv(t)
	env.repo.err = catalog.ErrCodeExists

	rec := env.call(t, user.RoleUretimPlanlama, "POST", "/materials", validMaterialBody)

	if rec.Code != http.StatusConflict {
		t.Fatalf("409 beklenirdi, gelen %d", rec.Code)
	}
	if code := decodeError(t, rec).Error.Code; code != "code_exists" {
		t.Errorf("code_exists beklenirdi, gelen %q", code)
	}
}

func TestCreateProduct_Success(t *testing.T) {
	env := newCatalogEnv(t)

	rec := env.call(t, user.RoleUretimPlanlama, "POST", "/products", validProductBody)

	if rec.Code != http.StatusCreated {
		t.Fatalf("201 beklenirdi, gelen %d", rec.Code)
	}
	var resp productResponse
	decodeInto(t, rec, &resp)
	if resp.ID != 4 || resp.Code != "P-1" || len(resp.BOM) != 1 ||
		resp.BOM[0].MaterialID != 1 || resp.BOM[0].QtyPerUnit != 5 {
		t.Errorf("beklenmeyen yanıt: %+v", resp)
	}
}

func TestCreateProduct_Validation(t *testing.T) {
	cases := map[string]string{
		"reçete yok":       `{"code":"P","name":"Ürün"}`,
		"boş reçete":       `{"code":"P","name":"Ürün","bom":[]}`,
		"sıfır miktar":     `{"code":"P","name":"Ürün","bom":[{"material_id":1,"qty_per_unit":0}]}`,
		"sıfır malzeme":    `{"code":"P","name":"Ürün","bom":[{"material_id":0,"qty_per_unit":1}]}`,
		"tekrarlı malzeme": `{"code":"P","name":"Ürün","bom":[{"material_id":1,"qty_per_unit":1},{"material_id":1,"qty_per_unit":2}]}`,
		"boş kod":          `{"code":"","name":"Ürün","bom":[{"material_id":1,"qty_per_unit":1}]}`,
	}

	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			env := newCatalogEnv(t)

			rec := env.call(t, user.RoleUretimPlanlama, "POST", "/products", body)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("400 beklenirdi, gelen %d", rec.Code)
			}
			if code := decodeError(t, rec).Error.Code; code != "validation_error" {
				t.Errorf("validation_error beklenirdi, gelen %q", code)
			}
			if env.repo.createdProductCode != "" {
				t.Error("geçersiz istekle ürün oluşturulmamalı")
			}
		})
	}
}

func TestCreateProduct_UnknownMaterial(t *testing.T) {
	env := newCatalogEnv(t)
	env.repo.err = catalog.ErrMaterialNotFound

	rec := env.call(t, user.RoleUretimPlanlama, "POST", "/products", validProductBody)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("422 beklenirdi, gelen %d", rec.Code)
	}
	if code := decodeError(t, rec).Error.Code; code != "material_not_found" {
		t.Errorf("material_not_found beklenirdi, gelen %q", code)
	}
}

func TestCatalog_UnexpectedErrorDoesNotLeakDetails(t *testing.T) {
	env := newCatalogEnv(t)
	env.repo.err = errors.New("bağlantı koptu: 10.0.0.7")

	rec := env.call(t, user.RoleUretimPlanlama, "POST", "/materials", validMaterialBody)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("500 beklenirdi, gelen %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "10.0.0.7") {
		t.Error("iç hata ayrıntısı kullanıcıya sızmamalı")
	}
}

func TestCatalog_EmptyListsAreArrays(t *testing.T) {
	for _, path := range []string{"/materials", "/products"} {
		t.Run(path, func(t *testing.T) {
			rec := newCatalogEnv(t).call(t, user.RoleBakim, "GET", path, "")

			if rec.Code != http.StatusOK {
				t.Fatalf("200 beklenirdi, gelen %d", rec.Code)
			}
			if body := strings.TrimSpace(rec.Body.String()); body != "[]" {
				t.Errorf("boş liste [] olmalı, gelen %s", body)
			}
		})
	}
}

func TestListProducts_BOMIsNeverNull(t *testing.T) {
	env := newCatalogEnv(t)
	env.repo.products = []catalog.Product{{ID: 1, Code: "P", Name: "Ürün"}}

	rec := env.call(t, user.RoleBakim, "GET", "/products", "")

	if !strings.Contains(rec.Body.String(), `"bom":[]`) {
		t.Errorf("reçetesiz ürün için \"bom\":[] beklenirdi, gövde: %s", rec.Body.String())
	}
}
