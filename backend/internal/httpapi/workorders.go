package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/idempotency"
	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/user"
	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/workorder"
)

const (
	defaultOrderPageSize = 20
	maxOrderPageSize     = 100
)

// WorkOrderService, iş emri işlemlerini yapan her şeyi temsil eder.
type WorkOrderService interface {
	Release(ctx context.Context, id int64) (workorder.WorkOrder, error)
	Cancel(ctx context.Context, id int64) (workorder.WorkOrder, error)
	RecordProduction(ctx context.Context, id, good, scrap, rework, userID int64) (workorder.WorkOrder, error)
	RecordRework(ctx context.Context, id, good, scrap, userID int64) (workorder.WorkOrder, error)
	Close(ctx context.Context, id int64, allowPartial bool) (workorder.WorkOrder, error)
}

// WorkOrdersAPI, iş emri uçlarını sunar.
type WorkOrdersAPI struct {
	service WorkOrderService
	repo    workorder.Repository
	idem    IdempotencyStore
}

// WithIdempotency, kritik POST uçlarını Idempotency-Key desteğiyle sarar.
func (a *WorkOrdersAPI) WithIdempotency(store IdempotencyStore) *WorkOrdersAPI {
	a.idem = store
	return a
}

// idempotent, store verilmişse handler'ı Idempotent ara katmanıyla sarar.
func (a *WorkOrdersAPI) idempotent(h http.HandlerFunc) http.HandlerFunc {
	if a.idem == nil {
		return h
	}
	return Idempotent(a.idem, h)
}

// NewWorkOrdersAPI, yeni bir WorkOrdersAPI oluşturur.
func NewWorkOrdersAPI(service WorkOrderService, repo workorder.Repository) *WorkOrdersAPI {
	return &WorkOrdersAPI{service: service, repo: repo}
}

// Register, iş emri uçlarını rol kontrolleriyle birlikte mux'a ekler.
func (a *WorkOrdersAPI) Register(mux *http.ServeMux, authenticate func(http.Handler) http.Handler) {
	mux.Handle("POST /work-orders", protected(authenticate, a.idempotent(a.create),
		user.RoleUretimPlanlama, user.RoleFabrikaMuduru))
	mux.Handle("GET /work-orders", protected(authenticate, a.list))
	mux.Handle("GET /work-orders/{id}", protected(authenticate, a.get))
	mux.Handle("POST /work-orders/{id}/release", protected(authenticate, a.release,
		user.RoleUretimPlanlama, user.RoleHatSefi, user.RoleFabrikaMuduru))
	mux.Handle("POST /work-orders/{id}/cancel", protected(authenticate, a.cancel,
		user.RoleUretimPlanlama, user.RoleFabrikaMuduru))
	mux.Handle("POST /work-orders/{id}/production", protected(authenticate, a.production,
		user.RoleOperator, user.RoleHatSefi))
	mux.Handle("POST /work-orders/{id}/rework", protected(authenticate, a.rework,
		user.RoleOperator, user.RoleHatSefi, user.RoleKaliteMuhendisi))
	mux.Handle("POST /work-orders/{id}/close", protected(authenticate, a.closeOrder,
		user.RoleHatSefi, user.RoleFabrikaMuduru))
}

type createOrderRequest struct {
	ProductID  int64 `json:"product_id"`
	PlannedQty int64 `json:"planned_qty"`
}

type productionRequest struct {
	Good   int64 `json:"good"`
	Scrap  int64 `json:"scrap"`
	Rework int64 `json:"rework"`
}

type reworkRequest struct {
	Good  int64 `json:"good"`
	Scrap int64 `json:"scrap"`
}

type closeRequest struct {
	AllowPartial bool `json:"allow_partial"`
}

type orderResponse struct {
	ID            int64  `json:"id"`
	ProductID     int64  `json:"product_id"`
	PlannedQty    int64  `json:"planned_qty"`
	Status        string `json:"status"`
	StartedQty    int64  `json:"started_qty"`
	GoodQty       int64  `json:"good_qty"`
	ScrapQty      int64  `json:"scrap_qty"`
	ReworkPending int64  `json:"rework_pending"`
}

type orderListResponse struct {
	Items      []orderResponse `json:"items"`
	NextBefore *int64          `json:"next_before"`
}

func (a *WorkOrdersAPI) create(w http.ResponseWriter, r *http.Request) {
	claims, ok := ClaimsFrom(r.Context())
	if !ok {
		writeUnauthorized(w, "missing_token", "kimlik doğrulama gerekli")
		return
	}

	var req createOrderRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if req.ProductID <= 0 {
		WriteError(w, http.StatusBadRequest, "validation_error", "product_id pozitif bir tam sayı olmalı")
		return
	}

	order, err := a.repo.Create(r.Context(), req.ProductID, req.PlannedQty, claims.UserID)
	if err != nil {
		writeOrderError(w, err)
		return
	}
	WriteJSON(w, http.StatusCreated, newOrderResponse(order))
}

func (a *WorkOrdersAPI) list(w http.ResponseWriter, r *http.Request) {
	limit, before, ok := parsePaging(w, r)
	if !ok {
		return
	}

	// Bir fazla kayıt isteyerek sonraki sayfa olup olmadığını anlarız.
	orders, err := a.repo.List(r.Context(), before, limit+1)
	if err != nil {
		writeOrderError(w, err)
		return
	}

	response := orderListResponse{Items: make([]orderResponse, 0, len(orders))}
	if len(orders) > limit {
		orders = orders[:limit]
		next := orders[len(orders)-1].ID
		response.NextBefore = &next
	}
	for _, order := range orders {
		response.Items = append(response.Items, newOrderResponse(order))
	}
	WriteJSON(w, http.StatusOK, response)
}

func (a *WorkOrdersAPI) get(w http.ResponseWriter, r *http.Request) {
	id, ok := parseOrderID(w, r)
	if !ok {
		return
	}

	order, err := a.repo.Get(r.Context(), id)
	if err != nil {
		writeOrderError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, newOrderResponse(order))
}

func (a *WorkOrdersAPI) release(w http.ResponseWriter, r *http.Request) {
	a.transition(w, r, func(ctx context.Context, id int64) (workorder.WorkOrder, error) {
		return a.service.Release(ctx, id)
	})
}

func (a *WorkOrdersAPI) cancel(w http.ResponseWriter, r *http.Request) {
	a.transition(w, r, func(ctx context.Context, id int64) (workorder.WorkOrder, error) {
		return a.service.Cancel(ctx, id)
	})
}

// transition, gövdesi olmayan durum geçişleri için ortak akıştır.
func (a *WorkOrdersAPI) transition(
	w http.ResponseWriter,
	r *http.Request,
	change func(ctx context.Context, id int64) (workorder.WorkOrder, error),
) {
	id, ok := parseOrderID(w, r)
	if !ok {
		return
	}

	order, err := change(r.Context(), id)
	if err != nil {
		writeOrderError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, newOrderResponse(order))
}

func (a *WorkOrdersAPI) production(w http.ResponseWriter, r *http.Request) {
	id, ok := parseOrderID(w, r)
	if !ok {
		return
	}
	claims, ok := ClaimsFrom(r.Context())
	if !ok {
		writeUnauthorized(w, "missing_token", "kimlik doğrulama gerekli")
		return
	}

	var req productionRequest
	if !decodeBody(w, r, &req) {
		return
	}

	order, err := a.service.RecordProduction(r.Context(), id, req.Good, req.Scrap, req.Rework, claims.UserID)
	if err != nil {
		writeOrderError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, newOrderResponse(order))
}

func (a *WorkOrdersAPI) rework(w http.ResponseWriter, r *http.Request) {
	id, ok := parseOrderID(w, r)
	if !ok {
		return
	}
	claims, ok := ClaimsFrom(r.Context())
	if !ok {
		writeUnauthorized(w, "missing_token", "kimlik doğrulama gerekli")
		return
	}

	var req reworkRequest
	if !decodeBody(w, r, &req) {
		return
	}

	order, err := a.service.RecordRework(r.Context(), id, req.Good, req.Scrap, claims.UserID)
	if err != nil {
		writeOrderError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, newOrderResponse(order))
}

func (a *WorkOrdersAPI) closeOrder(w http.ResponseWriter, r *http.Request) {
	id, ok := parseOrderID(w, r)
	if !ok {
		return
	}

	var req closeRequest
	if r.ContentLength != 0 && !decodeBody(w, r, &req) {
		return
	}

	order, err := a.service.Close(r.Context(), id, req.AllowPartial)
	if err != nil {
		writeOrderError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, newOrderResponse(order))
}

func parseOrderID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		WriteError(w, http.StatusBadRequest, "invalid_id", "iş emri kimliği pozitif bir tam sayı olmalı")
		return 0, false
	}
	return id, true
}

// parsePaging, keyset sayfalama parametrelerini okur: limit (1-100, varsayılan 20) ve before (isteğe bağlı).
func parsePaging(w http.ResponseWriter, r *http.Request) (limit int, before int64, ok bool) {
	limit = defaultOrderPageSize

	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > maxOrderPageSize {
			WriteError(w, http.StatusBadRequest, "invalid_limit",
				"limit 1 ile "+strconv.Itoa(maxOrderPageSize)+" arasında olmalı")
			return 0, 0, false
		}
		limit = parsed
	}

	if raw := r.URL.Query().Get("before"); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed <= 0 {
			WriteError(w, http.StatusBadRequest, "invalid_cursor", "before pozitif bir tam sayı olmalı")
			return 0, 0, false
		}
		before = parsed
	}
	return limit, before, true
}

// writeOrderError, iş emri hatalarını uygun HTTP yanıtına çevirir.
// Beklenmeyen hataların ayrıntısı log'a yazılır, kullanıcıya gösterilmez.
func writeOrderError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, workorder.ErrOrderNotFound):
		WriteError(w, http.StatusNotFound, "order_not_found", "iş emri bulunamadı")
	case errors.Is(err, workorder.ErrProductNotFound):
		WriteError(w, http.StatusUnprocessableEntity, "product_not_found", "ürün bulunamadı")
	case errors.Is(err, workorder.ErrInvalidQuantity):
		WriteError(w, http.StatusBadRequest, "invalid_quantity", err.Error())
	case errors.Is(err, workorder.ErrInvalidTransition):
		WriteError(w, http.StatusConflict, "invalid_transition", err.Error())
	case errors.Is(err, workorder.ErrExceedsPlan):
		WriteError(w, http.StatusConflict, "exceeds_plan", err.Error())
	case errors.Is(err, workorder.ErrInsufficientStock):
		WriteError(w, http.StatusConflict, "insufficient_stock", err.Error())
	case errors.Is(err, workorder.ErrReworkPending):
		WriteError(w, http.StatusConflict, "rework_pending", err.Error())
	case errors.Is(err, workorder.ErrPartialNotAllowed):
		WriteError(w, http.StatusConflict, "partial_not_confirmed", err.Error())
	case errors.Is(err, workorder.ErrEmptyBOM), errors.Is(err, workorder.ErrInvalidBOM):
		WriteError(w, http.StatusUnprocessableEntity, "invalid_bom", err.Error())
	case errors.Is(err, idempotency.ErrInProgress):
		WriteError(w, http.StatusConflict, "in_progress", "aynı Idempotency-Key ile bir istek zaten işleniyor")
	default:
		slog.Error("iş emri işlemi başarısız", "error", err)
		WriteError(w, http.StatusInternalServerError, "internal_error", "beklenmeyen bir hata oluştu")
	}
}

func newOrderResponse(order workorder.WorkOrder) orderResponse {
	return orderResponse{
		ID:            order.ID,
		ProductID:     order.ProductID,
		PlannedQty:    order.PlannedQty,
		Status:        string(order.Status),
		StartedQty:    order.StartedQty,
		GoodQty:       order.GoodQty,
		ScrapQty:      order.ScrapQty,
		ReworkPending: order.ReworkPending,
	}
}
