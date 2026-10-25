package httpapi

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/catalog"
	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/user"
)

// CatalogAPI, malzeme ve ürün uçlarını sunar.
type CatalogAPI struct {
	repo catalog.Repository
}

// NewCatalogAPI, yeni bir CatalogAPI oluşturur.
func NewCatalogAPI(repo catalog.Repository) *CatalogAPI {
	return &CatalogAPI{repo: repo}
}

// Register, katalog uçlarını rol kontrolleriyle birlikte mux'a ekler.
func (a *CatalogAPI) Register(mux *http.ServeMux, authenticate func(http.Handler) http.Handler) {
	planners := []user.Role{user.RoleUretimPlanlama, user.RoleFabrikaMuduru}

	mux.Handle("POST /materials", protected(authenticate, a.createMaterial, planners...))
	mux.Handle("GET /materials", protected(authenticate, a.listMaterials))
	mux.Handle("POST /products", protected(authenticate, a.createProduct, planners...))
	mux.Handle("GET /products", protected(authenticate, a.listProducts))
}

// protected, uç noktayı kimlik doğrulamasının arkasına koyar.
// Rol verilirse yalnızca o rollere izin verilir.
func protected(
	authenticate func(http.Handler) http.Handler,
	handler http.HandlerFunc,
	roles ...user.Role,
) http.Handler {
	if len(roles) == 0 {
		return authenticate(handler)
	}
	return authenticate(RequireRoles(roles...)(handler))
}

type createMaterialRequest struct {
	Code     string `json:"code"`
	Name     string `json:"name"`
	StockQty int64  `json:"stock_qty"`
}

type bomItemRequest struct {
	MaterialID int64 `json:"material_id"`
	QtyPerUnit int64 `json:"qty_per_unit"`
}

type createProductRequest struct {
	Code string           `json:"code"`
	Name string           `json:"name"`
	BOM  []bomItemRequest `json:"bom"`
}

type materialResponse struct {
	ID       int64  `json:"id"`
	Code     string `json:"code"`
	Name     string `json:"name"`
	StockQty int64  `json:"stock_qty"`
}

type bomItemResponse struct {
	MaterialID int64 `json:"material_id"`
	QtyPerUnit int64 `json:"qty_per_unit"`
}

type productResponse struct {
	ID   int64             `json:"id"`
	Code string            `json:"code"`
	Name string            `json:"name"`
	BOM  []bomItemResponse `json:"bom"`
}

func (a *CatalogAPI) createMaterial(w http.ResponseWriter, r *http.Request) {
	var req createMaterialRequest
	if !decodeBody(w, r, &req) {
		return
	}

	code, name, err := catalog.ValidateMaterial(req.Code, req.Name, req.StockQty)
	if err != nil {
		writeCatalogError(w, err)
		return
	}

	material, err := a.repo.CreateMaterial(r.Context(), code, name, req.StockQty)
	if err != nil {
		writeCatalogError(w, err)
		return
	}
	WriteJSON(w, http.StatusCreated, newMaterialResponse(material))
}

func (a *CatalogAPI) listMaterials(w http.ResponseWriter, r *http.Request) {
	materials, err := a.repo.ListMaterials(r.Context())
	if err != nil {
		writeCatalogError(w, err)
		return
	}

	response := make([]materialResponse, 0, len(materials))
	for _, material := range materials {
		response = append(response, newMaterialResponse(material))
	}
	WriteJSON(w, http.StatusOK, response)
}

func (a *CatalogAPI) createProduct(w http.ResponseWriter, r *http.Request) {
	var req createProductRequest
	if !decodeBody(w, r, &req) {
		return
	}

	bom := make([]catalog.BOMItem, 0, len(req.BOM))
	for _, item := range req.BOM {
		bom = append(bom, catalog.BOMItem{MaterialID: item.MaterialID, QtyPerUnit: item.QtyPerUnit})
	}

	code, name, err := catalog.ValidateProduct(req.Code, req.Name, bom)
	if err != nil {
		writeCatalogError(w, err)
		return
	}

	product, err := a.repo.CreateProduct(r.Context(), code, name, bom)
	if err != nil {
		writeCatalogError(w, err)
		return
	}
	WriteJSON(w, http.StatusCreated, newProductResponse(product))
}

func (a *CatalogAPI) listProducts(w http.ResponseWriter, r *http.Request) {
	products, err := a.repo.ListProducts(r.Context())
	if err != nil {
		writeCatalogError(w, err)
		return
	}

	response := make([]productResponse, 0, len(products))
	for _, product := range products {
		response = append(response, newProductResponse(product))
	}
	WriteJSON(w, http.StatusOK, response)
}

// writeCatalogError, katalog hatalarını uygun HTTP yanıtına çevirir.
// Beklenmeyen hataların ayrıntısı log'a yazılır, kullanıcıya gösterilmez.
func writeCatalogError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, catalog.ErrInvalid):
		WriteError(w, http.StatusBadRequest, "validation_error", err.Error())
	case errors.Is(err, catalog.ErrCodeExists):
		WriteError(w, http.StatusConflict, "code_exists", "bu kodla bir kayıt zaten var")
	case errors.Is(err, catalog.ErrMaterialNotFound):
		WriteError(w, http.StatusUnprocessableEntity, "material_not_found", "reçetedeki malzemelerden biri bulunamadı")
	default:
		slog.Error("katalog işlemi başarısız", "error", err)
		WriteError(w, http.StatusInternalServerError, "internal_error", "beklenmeyen bir hata oluştu")
	}
}

func newMaterialResponse(material catalog.Material) materialResponse {
	return materialResponse{
		ID:       material.ID,
		Code:     material.Code,
		Name:     material.Name,
		StockQty: material.StockQty,
	}
}

func newProductResponse(product catalog.Product) productResponse {
	bom := make([]bomItemResponse, 0, len(product.BOM))
	for _, item := range product.BOM {
		bom = append(bom, bomItemResponse{MaterialID: item.MaterialID, QtyPerUnit: item.QtyPerUnit})
	}
	return productResponse{ID: product.ID, Code: product.Code, Name: product.Name, BOM: bom}
}
