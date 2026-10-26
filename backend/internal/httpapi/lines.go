package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"time"

	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/oee"
	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/user"
)

// OEEService, hat, duruş, sayaç ve OEE işlemlerini yapan her şeydir.
type OEEService interface {
	CreateLine(ctx context.Context, code, name string, idealCycle time.Duration) (oee.Line, error)
	ListLines(ctx context.Context) ([]oee.Line, error)
	Reasons(ctx context.Context) ([]oee.Reason, error)
	RecordDowntime(ctx context.Context, lineCode, reasonCode string, start, end time.Time, userID int64) (int64, error)
	RecordCount(ctx context.Context, lineCode string, at time.Time, good, scrap, userID int64) (int64, error)
	DayReport(ctx context.Context, lineCode string, day time.Time) (oee.DayReport, error)
}

// LinesAPI, hat, duruş, sayaç ve OEE uçlarını sunar.
type LinesAPI struct {
	service OEEService
}

// NewLinesAPI, yeni bir LinesAPI oluşturur.
func NewLinesAPI(service OEEService) *LinesAPI {
	return &LinesAPI{service: service}
}

// Register, uçları rol kontrolleriyle ekler.
func (a *LinesAPI) Register(mux *http.ServeMux, authenticate func(http.Handler) http.Handler) {
	mux.Handle("POST /lines", protected(authenticate, a.createLine, user.RoleUretimPlanlama, user.RoleFabrikaMuduru))
	mux.Handle("GET /lines", protected(authenticate, a.listLines))
	mux.Handle("GET /downtime-reasons", protected(authenticate, a.listReasons))
	mux.Handle("POST /lines/{code}/downtime", protected(authenticate, a.recordDowntime,
		user.RoleOperator, user.RoleHatSefi, user.RoleBakim))
	mux.Handle("POST /lines/{code}/counts", protected(authenticate, a.recordCount,
		user.RoleOperator, user.RoleHatSefi))
	mux.Handle("GET /lines/{code}/oee", protected(authenticate, a.dayReport))
}

type createLineRequest struct {
	Code         string `json:"code"`
	Name         string `json:"name"`
	IdealCycleMS int64  `json:"ideal_cycle_ms"`
}

type downtimeRequest struct {
	ReasonCode string    `json:"reason_code"`
	StartedAt  time.Time `json:"started_at"`
	EndedAt    time.Time `json:"ended_at"`
}

type countRequest struct {
	OccurredAt time.Time `json:"occurred_at"`
	Good       int64     `json:"good"`
	Scrap      int64     `json:"scrap"`
}

type lineResponseBody struct {
	ID           int64  `json:"id"`
	Code         string `json:"code"`
	Name         string `json:"name"`
	IdealCycleMS int64  `json:"ideal_cycle_ms"`
}

type reasonResponse struct {
	Code        string `json:"code"`
	Description string `json:"description"`
	Planned     bool   `json:"planned"`
}

type createdResponse struct {
	ID int64 `json:"id"`
}

type oeeResultResponse struct {
	Shift                string    `json:"shift,omitempty"`
	Start                time.Time `json:"start"`
	End                  time.Time `json:"end"`
	PlannedDowntimeMin   float64   `json:"planned_downtime_min"`
	UnplannedDowntimeMin float64   `json:"unplanned_downtime_min"`
	PlannedProductionMin float64   `json:"planned_production_min"`
	RunTimeMin           float64   `json:"run_time_min"`
	Good                 int64     `json:"good"`
	Scrap                int64     `json:"scrap"`
	Availability         float64   `json:"availability"`
	Performance          float64   `json:"performance"`
	Quality              float64   `json:"quality"`
	OEE                  float64   `json:"oee"`
}

type oeeReportResponse struct {
	Line   lineResponseBody    `json:"line"`
	Date   string              `json:"date"`
	Shifts []oeeResultResponse `json:"shifts"`
	Day    oeeResultResponse   `json:"day"`
}

func (a *LinesAPI) createLine(w http.ResponseWriter, r *http.Request) {
	var req createLineRequest
	if !decodeBody(w, r, &req) {
		return
	}
	line, err := a.service.CreateLine(r.Context(), req.Code, req.Name, time.Duration(req.IdealCycleMS)*time.Millisecond)
	if err != nil {
		writeOEEError(w, err)
		return
	}
	WriteJSON(w, http.StatusCreated, newLineResponseBody(line))
}

func (a *LinesAPI) listLines(w http.ResponseWriter, r *http.Request) {
	lines, err := a.service.ListLines(r.Context())
	if err != nil {
		writeOEEError(w, err)
		return
	}
	response := make([]lineResponseBody, 0, len(lines))
	for _, line := range lines {
		response = append(response, newLineResponseBody(line))
	}
	WriteJSON(w, http.StatusOK, response)
}

func (a *LinesAPI) listReasons(w http.ResponseWriter, r *http.Request) {
	reasons, err := a.service.Reasons(r.Context())
	if err != nil {
		writeOEEError(w, err)
		return
	}
	response := make([]reasonResponse, 0, len(reasons))
	for _, reason := range reasons {
		response = append(response, reasonResponse{Code: reason.Code, Description: reason.Description, Planned: reason.Planned})
	}
	WriteJSON(w, http.StatusOK, response)
}

func (a *LinesAPI) recordDowntime(w http.ResponseWriter, r *http.Request) {
	claims, ok := ClaimsFrom(r.Context())
	if !ok {
		writeUnauthorized(w, "missing_token", "kimlik doğrulama gerekli")
		return
	}
	var req downtimeRequest
	if !decodeBody(w, r, &req) {
		return
	}
	id, err := a.service.RecordDowntime(r.Context(), r.PathValue("code"), req.ReasonCode, req.StartedAt, req.EndedAt, claims.UserID)
	if err != nil {
		writeOEEError(w, err)
		return
	}
	WriteJSON(w, http.StatusCreated, createdResponse{ID: id})
}

func (a *LinesAPI) recordCount(w http.ResponseWriter, r *http.Request) {
	claims, ok := ClaimsFrom(r.Context())
	if !ok {
		writeUnauthorized(w, "missing_token", "kimlik doğrulama gerekli")
		return
	}
	var req countRequest
	if !decodeBody(w, r, &req) {
		return
	}
	id, err := a.service.RecordCount(r.Context(), r.PathValue("code"), req.OccurredAt, req.Good, req.Scrap, claims.UserID)
	if err != nil {
		writeOEEError(w, err)
		return
	}
	WriteJSON(w, http.StatusCreated, createdResponse{ID: id})
}

func (a *LinesAPI) dayReport(w http.ResponseWriter, r *http.Request) {
	day, err := time.Parse("2006-01-02", r.URL.Query().Get("date"))
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_date", "date parametresi YYYY-MM-DD biçiminde olmalı")
		return
	}
	report, err := a.service.DayReport(r.Context(), r.PathValue("code"), day)
	if err != nil {
		writeOEEError(w, err)
		return
	}

	response := oeeReportResponse{
		Line:   newLineResponseBody(report.Line),
		Date:   report.Day.Format("2006-01-02"),
		Shifts: make([]oeeResultResponse, 0, len(report.Shifts)),
		Day:    newOEEResult("", report.Total),
	}
	for _, shift := range report.Shifts {
		response.Shifts = append(response.Shifts, newOEEResult(shift.Name, shift.Result))
	}
	WriteJSON(w, http.StatusOK, response)
}

func writeOEEError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, oee.ErrInvalid):
		WriteError(w, http.StatusBadRequest, "validation_error", err.Error())
	case errors.Is(err, oee.ErrLineNotFound):
		WriteError(w, http.StatusNotFound, "line_not_found", "hat bulunamadı")
	case errors.Is(err, oee.ErrLineExists):
		WriteError(w, http.StatusConflict, "line_exists", "bu kodla bir hat zaten var")
	case errors.Is(err, oee.ErrUnknownReason):
		WriteError(w, http.StatusUnprocessableEntity, "unknown_reason", "duruş nedeni tanımlı değil")
	default:
		slog.Error("OEE işlemi başarısız", "error", err)
		WriteError(w, http.StatusInternalServerError, "internal_error", "beklenmeyen bir hata oluştu")
	}
}

func newLineResponseBody(line oee.Line) lineResponseBody {
	return lineResponseBody{ID: line.ID, Code: line.Code, Name: line.Name, IdealCycleMS: line.IdealCycle.Milliseconds()}
}

func round(value float64, places int) float64 {
	factor := math.Pow(10, float64(places))
	return math.Round(value*factor) / factor
}

func newOEEResult(name string, result oee.Result) oeeResultResponse {
	return oeeResultResponse{
		Shift:                name,
		Start:                result.Start,
		End:                  result.End,
		PlannedDowntimeMin:   round(result.PlannedDowntime.Minutes(), 2),
		UnplannedDowntimeMin: round(result.UnplannedDowntime.Minutes(), 2),
		PlannedProductionMin: round(result.PlannedProductionTime.Minutes(), 2),
		RunTimeMin:           round(result.RunTime.Minutes(), 2),
		Good:                 result.Good,
		Scrap:                result.Scrap,
		Availability:         round(result.Availability, 4),
		Performance:          round(result.Performance, 4),
		Quality:              round(result.Quality, 4),
		OEE:                  round(result.OEE, 4),
	}
}
