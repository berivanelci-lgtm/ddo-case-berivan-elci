package httpapi

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/ncr"
	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/user"
)

// NCRsAPI, uygunsuzluk (NCR) uçlarını sunar.
type NCRsAPI struct {
	repo ncr.Repository
	now  func() time.Time
}

// NewNCRsAPI, yeni bir NCRsAPI oluşturur.
func NewNCRsAPI(repo ncr.Repository) *NCRsAPI {
	return &NCRsAPI{repo: repo, now: time.Now}
}

// Register, uçları rol kontrolleriyle ekler.
func (a *NCRsAPI) Register(mux *http.ServeMux, authenticate func(http.Handler) http.Handler) {
	mux.Handle("POST /ncrs", protected(authenticate, a.create,
		user.RoleOperator, user.RoleHatSefi, user.RoleKaliteMuhendisi))
	mux.Handle("GET /ncrs", protected(authenticate, a.list))
	mux.Handle("GET /ncrs/{id}", protected(authenticate, a.get))
	mux.Handle("POST /ncrs/{id}/investigate", protected(authenticate, a.investigate, user.RoleKaliteMuhendisi))
	mux.Handle("POST /ncrs/{id}/close", protected(authenticate, a.closeNCR, user.RoleKaliteMuhendisi))
}

type createNCRRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Severity    string `json:"severity"`
	WorkOrderID *int64 `json:"work_order_id"`
	ChartID     *int64 `json:"chart_id"`
}

type closeNCRRequest struct {
	RootCause        string `json:"root_cause"`
	CorrectiveAction string `json:"corrective_action"`
}

type ncrResponse struct {
	ID               int64      `json:"id"`
	Title            string     `json:"title"`
	Description      string     `json:"description"`
	Severity         string     `json:"severity"`
	Status           string     `json:"status"`
	WorkOrderID      *int64     `json:"work_order_id"`
	ChartID          *int64     `json:"chart_id"`
	OpenedBy         int64      `json:"opened_by"`
	RootCause        *string    `json:"root_cause"`
	CorrectiveAction *string    `json:"corrective_action"`
	ClosedBy         *int64     `json:"closed_by"`
	CreatedAt        time.Time  `json:"created_at"`
	ClosedAt         *time.Time `json:"closed_at"`
}

type ncrListResponse struct {
	Items      []ncrResponse `json:"items"`
	NextBefore *int64        `json:"next_before"`
}

func (a *NCRsAPI) create(w http.ResponseWriter, r *http.Request) {
	claims, ok := ClaimsFrom(r.Context())
	if !ok {
		writeUnauthorized(w, "missing_token", "kimlik doğrulama gerekli")
		return
	}
	var req createNCRRequest
	if !decodeBody(w, r, &req) {
		return
	}

	n, err := ncr.New(req.Title, req.Description, ncr.Severity(req.Severity), req.WorkOrderID, req.ChartID, claims.UserID)
	if err != nil {
		writeNCRError(w, err)
		return
	}
	created, err := a.repo.Create(r.Context(), n)
	if err != nil {
		writeNCRError(w, err)
		return
	}
	WriteJSON(w, http.StatusCreated, newNCRResponse(created))
}

func (a *NCRsAPI) list(w http.ResponseWriter, r *http.Request) {
	limit, before, ok := parsePaging(w, r)
	if !ok {
		return
	}
	status := ncr.Status(r.URL.Query().Get("status"))
	if status != "" && !status.Valid() {
		WriteError(w, http.StatusBadRequest, "invalid_status", "status open, investigating ya da closed olmalı")
		return
	}

	items, err := a.repo.List(r.Context(), status, before, limit+1)
	if err != nil {
		writeNCRError(w, err)
		return
	}
	response := ncrListResponse{Items: make([]ncrResponse, 0, len(items))}
	if len(items) > limit {
		items = items[:limit]
		next := items[len(items)-1].ID
		response.NextBefore = &next
	}
	for _, item := range items {
		response.Items = append(response.Items, newNCRResponse(item))
	}
	WriteJSON(w, http.StatusOK, response)
}

func (a *NCRsAPI) get(w http.ResponseWriter, r *http.Request) {
	id, ok := parseNCRID(w, r)
	if !ok {
		return
	}
	n, err := a.repo.Get(r.Context(), id)
	if err != nil {
		writeNCRError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, newNCRResponse(n))
}

func (a *NCRsAPI) investigate(w http.ResponseWriter, r *http.Request) {
	id, ok := parseNCRID(w, r)
	if !ok {
		return
	}
	updated, err := a.repo.Update(r.Context(), id, func(n ncr.NCR) (ncr.NCR, error) {
		return n.StartInvestigation()
	})
	if err != nil {
		writeNCRError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, newNCRResponse(updated))
}

func (a *NCRsAPI) closeNCR(w http.ResponseWriter, r *http.Request) {
	claims, ok := ClaimsFrom(r.Context())
	if !ok {
		writeUnauthorized(w, "missing_token", "kimlik doğrulama gerekli")
		return
	}
	id, ok := parseNCRID(w, r)
	if !ok {
		return
	}
	var req closeNCRRequest
	if !decodeBody(w, r, &req) {
		return
	}

	updated, err := a.repo.Update(r.Context(), id, func(n ncr.NCR) (ncr.NCR, error) {
		return n.Close(req.RootCause, req.CorrectiveAction, claims.UserID, a.now())
	})
	if err != nil {
		writeNCRError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, newNCRResponse(updated))
}

func parseNCRID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		WriteError(w, http.StatusBadRequest, "invalid_id", "kayıt kimliği pozitif bir tam sayı olmalı")
		return 0, false
	}
	return id, true
}

func writeNCRError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ncr.ErrInvalid):
		WriteError(w, http.StatusBadRequest, "validation_error", err.Error())
	case errors.Is(err, ncr.ErrInvalidTransition):
		WriteError(w, http.StatusConflict, "invalid_transition", err.Error())
	case errors.Is(err, ncr.ErrNotFound):
		WriteError(w, http.StatusNotFound, "ncr_not_found", "uygunsuzluk kaydı bulunamadı")
	default:
		slog.Error("uygunsuzluk işlemi başarısız", "error", err)
		WriteError(w, http.StatusInternalServerError, "internal_error", "beklenmeyen bir hata oluştu")
	}
}

func newNCRResponse(n ncr.NCR) ncrResponse {
	response := ncrResponse{
		ID: n.ID, Title: n.Title, Description: n.Description,
		Severity: string(n.Severity), Status: string(n.Status),
		WorkOrderID: n.WorkOrderID, ChartID: n.ChartID, OpenedBy: n.OpenedBy,
		ClosedBy: n.ClosedBy, CreatedAt: n.CreatedAt, ClosedAt: n.ClosedAt,
	}
	if n.RootCause != "" {
		cause := n.RootCause
		response.RootCause = &cause
	}
	if n.CorrectiveAction != "" {
		action := n.CorrectiveAction
		response.CorrectiveAction = &action
	}
	return response
}
