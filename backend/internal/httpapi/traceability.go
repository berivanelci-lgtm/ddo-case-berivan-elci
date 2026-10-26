package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/genealogy"
	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/user"
)

// TraceabilityService, izlenebilirlik sorgularını yapan her şeydir.
type TraceabilityService interface {
	Backward(ctx context.Context, code string) (genealogy.Lot, []genealogy.Node, error)
	Forward(ctx context.Context, code string) (genealogy.Lot, []genealogy.Node, error)
	Recall(ctx context.Context, code string) (genealogy.RecallReport, error)
}

// TraceabilityAPI, parti/seri izlenebilirlik ve geri çağırma uçlarını sunar.
type TraceabilityAPI struct {
	service TraceabilityService
}

// NewTraceabilityAPI, yeni bir TraceabilityAPI oluşturur.
func NewTraceabilityAPI(service TraceabilityService) *TraceabilityAPI {
	return &TraceabilityAPI{service: service}
}

// Register, uçları ekler. Kalite, planlama, hat şefi ve fabrika müdürü okuyabilir.
func (a *TraceabilityAPI) Register(mux *http.ServeMux, authenticate func(http.Handler) http.Handler) {
	readers := []user.Role{user.RoleKaliteMuhendisi, user.RoleUretimPlanlama, user.RoleHatSefi, user.RoleFabrikaMuduru}

	mux.Handle("GET /traceability/lots/{code}/backward", protected(authenticate, a.backward, readers...))
	mux.Handle("GET /traceability/lots/{code}/forward", protected(authenticate, a.forward, readers...))
	mux.Handle("GET /traceability/lots/{code}/recall", protected(authenticate, a.recall, readers...))
}

type lotResponse struct {
	ID   int64  `json:"id"`
	Code string `json:"code"`
	Kind string `json:"kind"`
}

type nodeResponse struct {
	ID    int64  `json:"id"`
	Code  string `json:"code"`
	Kind  string `json:"kind"`
	Depth int    `json:"depth"`
}

type traceResponse struct {
	Lot  lotResponse    `json:"lot"`
	Lots []nodeResponse `json:"lots"`
}

type shipmentResponse struct {
	Code      string    `json:"code"`
	Customer  string    `json:"customer"`
	ShippedAt time.Time `json:"shipped_at"`
	Units     int64     `json:"affected_units"`
}

type recallResponse struct {
	Lot                 lotResponse        `json:"lot"`
	AffectedSerialCount int                `json:"affected_serial_count"`
	AffectedSerials     []string           `json:"affected_serials"`
	Customers           []string           `json:"customers"`
	Shipments           []shipmentResponse `json:"shipments"`
}

func (a *TraceabilityAPI) backward(w http.ResponseWriter, r *http.Request) {
	lot, nodes, err := a.service.Backward(r.Context(), r.PathValue("code"))
	if err != nil {
		writeTraceError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, newTraceResponse(lot, nodes))
}

func (a *TraceabilityAPI) forward(w http.ResponseWriter, r *http.Request) {
	lot, nodes, err := a.service.Forward(r.Context(), r.PathValue("code"))
	if err != nil {
		writeTraceError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, newTraceResponse(lot, nodes))
}

func (a *TraceabilityAPI) recall(w http.ResponseWriter, r *http.Request) {
	report, err := a.service.Recall(r.Context(), r.PathValue("code"))
	if err != nil {
		writeTraceError(w, err)
		return
	}

	response := recallResponse{
		Lot:                 newLotResponse(report.Lot),
		AffectedSerialCount: len(report.AffectedSerials),
		AffectedSerials:     append([]string{}, report.AffectedSerials...),
		Customers:           append([]string{}, report.Customers...),
		Shipments:           make([]shipmentResponse, 0, len(report.Shipments)),
	}
	for _, shipment := range report.Shipments {
		response.Shipments = append(response.Shipments, shipmentResponse{
			Code:      shipment.Code,
			Customer:  shipment.Customer,
			ShippedAt: shipment.ShippedAt,
			Units:     shipment.Units,
		})
	}
	WriteJSON(w, http.StatusOK, response)
}

func writeTraceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, genealogy.ErrInvalidCode):
		WriteError(w, http.StatusBadRequest, "invalid_code", err.Error())
	case errors.Is(err, genealogy.ErrLotNotFound):
		WriteError(w, http.StatusNotFound, "lot_not_found", "parti ya da seri bulunamadı")
	default:
		slog.Error("izlenebilirlik sorgusu başarısız", "error", err)
		WriteError(w, http.StatusInternalServerError, "internal_error", "beklenmeyen bir hata oluştu")
	}
}

func newLotResponse(lot genealogy.Lot) lotResponse {
	return lotResponse{ID: lot.ID, Code: lot.Code, Kind: string(lot.Kind)}
}

func newTraceResponse(lot genealogy.Lot, nodes []genealogy.Node) traceResponse {
	response := traceResponse{Lot: newLotResponse(lot), Lots: make([]nodeResponse, 0, len(nodes))}
	for _, node := range nodes {
		response.Lots = append(response.Lots, nodeResponse{
			ID: node.ID, Code: node.Code, Kind: string(node.Kind), Depth: node.Depth,
		})
	}
	return response
}
