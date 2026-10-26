package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/quality"
	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/user"
)

const recentInspectionLimit = 50

// QualityService, kalite formu ve muayene işlemlerini yapan her şeydir.
type QualityService interface {
	CreateForm(ctx context.Context, f quality.Form, createdBy int64) (quality.Form, error)
	ListForms(ctx context.Context) ([]quality.Form, error)
	GetForm(ctx context.Context, id int64) (quality.Form, error)
	RecentInspections(ctx context.Context, formID int64, limit int) ([]quality.Inspection, error)
	Inspect(ctx context.Context, formID int64, values []float64, workOrderID *int64, userID int64) (quality.Inspection, []quality.ChartOutcome, error)
}

// QualityAPI, kalite ölçüm formu uçlarını sunar.
type QualityAPI struct {
	service QualityService
}

// NewQualityAPI, yeni bir QualityAPI oluşturur.
func NewQualityAPI(service QualityService) *QualityAPI {
	return &QualityAPI{service: service}
}

// Register, uçları rol kontrolleriyle ekler.
func (a *QualityAPI) Register(mux *http.ServeMux, authenticate func(http.Handler) http.Handler) {
	mux.Handle("POST /quality-forms", protected(authenticate, a.createForm, user.RoleKaliteMuhendisi))
	mux.Handle("GET /quality-forms", protected(authenticate, a.listForms))
	mux.Handle("GET /quality-forms/{id}", protected(authenticate, a.getForm))
	mux.Handle("GET /quality-forms/{id}/inspections", protected(authenticate, a.listInspections))
	mux.Handle("POST /quality-forms/{id}/inspections", protected(authenticate, a.inspect,
		user.RoleOperator, user.RoleHatSefi, user.RoleKaliteMuhendisi))
}

type formItemBody struct {
	Characteristic string  `json:"characteristic"`
	Unit           string  `json:"unit"`
	Nominal        float64 `json:"nominal"`
	LSL            float64 `json:"lsl"`
	USL            float64 `json:"usl"`
	ChartID        *int64  `json:"chart_id"`
}

type createFormRequest struct {
	Code           string         `json:"code"`
	Name           string         `json:"name"`
	GaugeCode      string         `json:"gauge_code"`
	CalibrationDue string         `json:"calibration_due"`
	Items          []formItemBody `json:"items"`
}

type inspectRequest struct {
	Values      []float64 `json:"values"`
	WorkOrderID *int64    `json:"work_order_id"`
}

type formItemResponse struct {
	Position int `json:"position"`
	formItemBody
}

type formResponse struct {
	ID             int64              `json:"id"`
	Code           string             `json:"code"`
	Name           string             `json:"name"`
	GaugeCode      string             `json:"gauge_code"`
	CalibrationDue *string            `json:"calibration_due"`
	Items          []formItemResponse `json:"items"`
}

type valueResponse struct {
	Position       int     `json:"position"`
	Characteristic string  `json:"characteristic"`
	Value          float64 `json:"value"`
	InSpec         bool    `json:"in_spec"`
}

type inspectionResponse struct {
	ID          int64           `json:"id"`
	FormID      int64           `json:"form_id"`
	InspectedBy int64           `json:"inspected_by"`
	InspectedAt time.Time       `json:"inspected_at"`
	WorkOrderID *int64          `json:"work_order_id"`
	AllInSpec   bool            `json:"all_in_spec"`
	Values      []valueResponse `json:"values"`
}

type chartOutcomeResponse struct {
	Position int             `json:"position"`
	ChartID  int64           `json:"chart_id"`
	Seq      int64           `json:"seq"`
	Alarms   []alarmResponse `json:"alarms"`
	Error    string          `json:"error,omitempty"`
}

type inspectResponse struct {
	inspectionResponse
	SPC []chartOutcomeResponse `json:"spc"`
}

func (a *QualityAPI) createForm(w http.ResponseWriter, r *http.Request) {
	claims, ok := ClaimsFrom(r.Context())
	if !ok {
		writeUnauthorized(w, "missing_token", "kimlik doğrulama gerekli")
		return
	}
	var req createFormRequest
	if !decodeBody(w, r, &req) {
		return
	}

	form := quality.Form{Code: req.Code, Name: req.Name, GaugeCode: req.GaugeCode}
	if req.CalibrationDue != "" {
		due, err := time.Parse("2006-01-02", req.CalibrationDue)
		if err != nil {
			WriteError(w, http.StatusBadRequest, "validation_error", "calibration_due YYYY-MM-DD biçiminde olmalı")
			return
		}
		form.CalibrationDue = &due
	}
	for _, item := range req.Items {
		form.Items = append(form.Items, quality.Item{
			Characteristic: item.Characteristic, Unit: item.Unit,
			Nominal: item.Nominal, LSL: item.LSL, USL: item.USL, ChartID: item.ChartID,
		})
	}

	created, err := a.service.CreateForm(r.Context(), form, claims.UserID)
	if err != nil {
		writeQualityError(w, err)
		return
	}
	WriteJSON(w, http.StatusCreated, newFormResponse(created))
}

func (a *QualityAPI) listForms(w http.ResponseWriter, r *http.Request) {
	forms, err := a.service.ListForms(r.Context())
	if err != nil {
		writeQualityError(w, err)
		return
	}
	response := make([]formResponse, 0, len(forms))
	for _, f := range forms {
		response = append(response, newFormResponse(f))
	}
	WriteJSON(w, http.StatusOK, response)
}

func (a *QualityAPI) getForm(w http.ResponseWriter, r *http.Request) {
	id, ok := parseFormID(w, r)
	if !ok {
		return
	}
	form, err := a.service.GetForm(r.Context(), id)
	if err != nil {
		writeQualityError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, newFormResponse(form))
}

func (a *QualityAPI) listInspections(w http.ResponseWriter, r *http.Request) {
	id, ok := parseFormID(w, r)
	if !ok {
		return
	}
	inspections, err := a.service.RecentInspections(r.Context(), id, recentInspectionLimit)
	if err != nil {
		writeQualityError(w, err)
		return
	}
	response := make([]inspectionResponse, 0, len(inspections))
	for _, in := range inspections {
		response = append(response, newInspectionResponse(in))
	}
	WriteJSON(w, http.StatusOK, response)
}

func (a *QualityAPI) inspect(w http.ResponseWriter, r *http.Request) {
	claims, ok := ClaimsFrom(r.Context())
	if !ok {
		writeUnauthorized(w, "missing_token", "kimlik doğrulama gerekli")
		return
	}
	id, ok := parseFormID(w, r)
	if !ok {
		return
	}
	var req inspectRequest
	if !decodeBody(w, r, &req) {
		return
	}

	saved, outcomes, err := a.service.Inspect(r.Context(), id, req.Values, req.WorkOrderID, claims.UserID)
	if err != nil {
		writeQualityError(w, err)
		return
	}
	response := inspectResponse{inspectionResponse: newInspectionResponse(saved), SPC: make([]chartOutcomeResponse, 0, len(outcomes))}
	for _, o := range outcomes {
		response.SPC = append(response.SPC, chartOutcomeResponse{
			Position: o.Position, ChartID: o.ChartID, Seq: o.Seq, Alarms: newAlarmResponses(o.Alarms), Error: o.Error,
		})
	}
	WriteJSON(w, http.StatusCreated, response)
}

func parseFormID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		WriteError(w, http.StatusBadRequest, "invalid_id", "form kimliği pozitif bir tam sayı olmalı")
		return 0, false
	}
	return id, true
}

func writeQualityError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, quality.ErrInvalid):
		WriteError(w, http.StatusBadRequest, "validation_error", err.Error())
	case errors.Is(err, quality.ErrNotFound):
		WriteError(w, http.StatusNotFound, "form_not_found", "kalite formu bulunamadı")
	case errors.Is(err, quality.ErrExists):
		WriteError(w, http.StatusConflict, "form_exists", "bu kodla bir kalite formu zaten var")
	case errors.Is(err, quality.ErrCalibrationExpired):
		WriteError(w, http.StatusConflict, "calibration_expired", err.Error())
	default:
		slog.Error("kalite işlemi başarısız", "error", err)
		WriteError(w, http.StatusInternalServerError, "internal_error", "beklenmeyen bir hata oluştu")
	}
}

func newFormResponse(f quality.Form) formResponse {
	response := formResponse{ID: f.ID, Code: f.Code, Name: f.Name, GaugeCode: f.GaugeCode, Items: make([]formItemResponse, 0, len(f.Items))}
	if f.CalibrationDue != nil {
		due := f.CalibrationDue.Format("2006-01-02")
		response.CalibrationDue = &due
	}
	for _, item := range f.Items {
		response.Items = append(response.Items, formItemResponse{
			Position: item.Position,
			formItemBody: formItemBody{
				Characteristic: item.Characteristic, Unit: item.Unit,
				Nominal: item.Nominal, LSL: item.LSL, USL: item.USL, ChartID: item.ChartID,
			},
		})
	}
	return response
}

func newInspectionResponse(in quality.Inspection) inspectionResponse {
	response := inspectionResponse{
		ID: in.ID, FormID: in.FormID, InspectedBy: in.InspectedBy, InspectedAt: in.InspectedAt,
		WorkOrderID: in.WorkOrderID, AllInSpec: in.AllInSpec, Values: make([]valueResponse, 0, len(in.Values)),
	}
	for _, v := range in.Values {
		response.Values = append(response.Values, valueResponse{
			Position: v.Position, Characteristic: v.Characteristic, Value: v.Value, InSpec: v.InSpec,
		})
	}
	return response
}
