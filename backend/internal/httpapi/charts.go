package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/spc"
	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/user"
)

const (
	maxChartNameLength = 100
	detailPointLimit   = 100
	detailAlarmLimit   = 50
)

// ChartService, ölçüm ekleme ve limit yenileme işlemlerini yapan her şeyi temsil eder.
type ChartService interface {
	AddMeasurement(ctx context.Context, chartID int64, subgroup []float64) (spc.Result, error)
	RecomputeLimits(ctx context.Context, chartID int64) (spc.LimitSet, error)
}

// ChartsAPI, kontrol grafiği uçlarını sunar.
type ChartsAPI struct {
	service ChartService
	repo    spc.ChartRepository
}

// NewChartsAPI, yeni bir ChartsAPI oluşturur.
func NewChartsAPI(service ChartService, repo spc.ChartRepository) *ChartsAPI {
	return &ChartsAPI{service: service, repo: repo}
}

// Register, grafik uçlarını rol kontrolleriyle birlikte mux'a ekler.
func (a *ChartsAPI) Register(mux *http.ServeMux, authenticate func(http.Handler) http.Handler) {
	protect := func(handler http.HandlerFunc, roles ...user.Role) http.Handler {
		if len(roles) == 0 {
			return authenticate(handler)
		}
		return authenticate(RequireRoles(roles...)(handler))
	}

	mux.Handle("POST /charts", protect(a.create,
		user.RoleKaliteMuhendisi, user.RoleHatSefi, user.RoleFabrikaMuduru))
	mux.Handle("GET /charts", protect(a.list))
	mux.Handle("GET /charts/{id}", protect(a.get))
	mux.Handle("POST /charts/{id}/measurements", protect(a.addMeasurement,
		user.RoleOperator, user.RoleHatSefi, user.RoleKaliteMuhendisi))
	mux.Handle("POST /charts/{id}/limits/recompute", protect(a.recomputeLimits,
		user.RoleKaliteMuhendisi))
}

type createChartRequest struct {
	Name         string `json:"name"`
	Type         string `json:"type"`
	SubgroupSize int    `json:"subgroup_size"`
}

type measurementRequest struct {
	Values []float64 `json:"values"`
}

type chartResponse struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	Type         string `json:"type"`
	SubgroupSize int    `json:"subgroup_size"`
}

type rangeLimitsResponse struct {
	Center float64 `json:"center"`
	UCL    float64 `json:"ucl"`
	LCL    float64 `json:"lcl"`
}

type limitsResponse struct {
	Version int                  `json:"version"`
	Center  float64              `json:"center"`
	Sigma   float64              `json:"sigma"`
	UCL     float64              `json:"ucl"`
	LCL     float64              `json:"lcl"`
	Range   *rangeLimitsResponse `json:"range"`
}

type pointResponse struct {
	Seq       int64     `json:"seq"`
	Values    []float64 `json:"values"`
	Statistic float64   `json:"statistic"`
}

type alarmResponse struct {
	PointSeq     int64   `json:"point_seq"`
	Rule         string  `json:"rule"`
	Value        float64 `json:"value"`
	LimitVersion int     `json:"limit_version"`
}

type chartDetailResponse struct {
	Chart  chartResponse   `json:"chart"`
	Limits *limitsResponse `json:"limits"`
	Points []pointResponse `json:"points"`
	Alarms []alarmResponse `json:"alarms"`
}

type measurementResponse struct {
	Seq            int64           `json:"seq"`
	Statistic      float64         `json:"statistic"`
	LimitsComputed bool            `json:"limits_computed"`
	LimitVersion   int             `json:"limit_version"`
	Limits         *limitsResponse `json:"limits"`
	Alarms         []alarmResponse `json:"alarms"`
}

func (a *ChartsAPI) create(w http.ResponseWriter, r *http.Request) {
	claims, ok := ClaimsFrom(r.Context())
	if !ok {
		writeUnauthorized(w, "missing_token", "kimlik doğrulama gerekli")
		return
	}

	var req createChartRequest
	if !decodeBody(w, r, &req) {
		return
	}

	name, chartType, size, problem := validateCreateChart(req)
	if problem != "" {
		WriteError(w, http.StatusBadRequest, "validation_error", problem)
		return
	}

	chart, err := a.repo.CreateChart(r.Context(), name, chartType, size, claims.UserID)
	if err != nil {
		writeSPCError(w, err)
		return
	}
	WriteJSON(w, http.StatusCreated, newChartResponse(chart))
}

func (a *ChartsAPI) list(w http.ResponseWriter, r *http.Request) {
	charts, err := a.repo.ListCharts(r.Context())
	if err != nil {
		writeSPCError(w, err)
		return
	}

	response := make([]chartResponse, 0, len(charts))
	for _, chart := range charts {
		response = append(response, newChartResponse(chart))
	}
	WriteJSON(w, http.StatusOK, response)
}

func (a *ChartsAPI) get(w http.ResponseWriter, r *http.Request) {
	id, ok := parseChartID(w, r)
	if !ok {
		return
	}

	chart, err := a.repo.GetChart(r.Context(), id)
	if err != nil {
		writeSPCError(w, err)
		return
	}
	limits, err := a.repo.LatestLimits(r.Context(), id)
	if err != nil {
		writeSPCError(w, err)
		return
	}
	points, err := a.repo.RecentPoints(r.Context(), id, detailPointLimit)
	if err != nil {
		writeSPCError(w, err)
		return
	}
	alarms, err := a.repo.RecentAlarms(r.Context(), id, detailAlarmLimit)
	if err != nil {
		writeSPCError(w, err)
		return
	}

	detail := chartDetailResponse{
		Chart:  newChartResponse(chart),
		Points: newPointResponses(points),
		Alarms: newAlarmResponses(alarms),
	}
	if limits != nil {
		detail.Limits = newLimitsResponse(*limits)
	}
	WriteJSON(w, http.StatusOK, detail)
}

func (a *ChartsAPI) addMeasurement(w http.ResponseWriter, r *http.Request) {
	id, ok := parseChartID(w, r)
	if !ok {
		return
	}

	var req measurementRequest
	if !decodeBody(w, r, &req) {
		return
	}

	result, err := a.service.AddMeasurement(r.Context(), id, req.Values)
	if err != nil {
		writeSPCError(w, err)
		return
	}

	response := measurementResponse{
		Seq:            result.Seq,
		Statistic:      result.Statistic,
		LimitsComputed: result.LimitsComputed,
		LimitVersion:   result.LimitVersion,
		Alarms:         newAlarmResponses(result.Alarms),
	}
	if result.Limits != nil {
		response.Limits = newLimitsResponse(*result.Limits)
	}
	WriteJSON(w, http.StatusCreated, response)
}

func (a *ChartsAPI) recomputeLimits(w http.ResponseWriter, r *http.Request) {
	id, ok := parseChartID(w, r)
	if !ok {
		return
	}

	set, err := a.service.RecomputeLimits(r.Context(), id)
	if err != nil {
		writeSPCError(w, err)
		return
	}
	WriteJSON(w, http.StatusCreated, newLimitsResponse(set))
}

// validateCreateChart, grafik oluşturma isteğini doğrular ve temizlenmiş değerleri döner.
// Sorun varsa problem alanı dolu gelir.
func validateCreateChart(req createChartRequest) (name string, chartType spc.ChartType, size int, problem string) {
	name = strings.TrimSpace(req.Name)
	if name == "" || utf8.RuneCountInString(name) > maxChartNameLength {
		return "", "", 0, "ad 1 ile 100 karakter arasında olmalı"
	}

	chartType = spc.ChartType(req.Type)
	switch chartType {
	case spc.ChartIndividual:
		if req.SubgroupSize != 0 && req.SubgroupSize != 1 {
			return "", "", 0, "bireysel grafikte alt grup büyüklüğü 1 olmalı"
		}
		return name, chartType, 1, ""
	case spc.ChartXbarR:
		if req.SubgroupSize < 2 || req.SubgroupSize > 10 {
			return "", "", 0, "X̄-R grafikte alt grup büyüklüğü 2 ile 10 arasında olmalı"
		}
		return name, chartType, req.SubgroupSize, ""
	default:
		return "", "", 0, "type alanı individual veya xbar_r olmalı"
	}
}

// decodeBody, istek gövdesini JSON olarak okur. Gövde 1 MB ile sınırlıdır ve bilinmeyen alanlar reddedilir.
func decodeBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(dst); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_request", "istek gövdesi geçerli bir JSON değil")
		return false
	}
	return true
}

func parseChartID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		WriteError(w, http.StatusBadRequest, "invalid_id", "grafik kimliği pozitif bir tam sayı olmalı")
		return 0, false
	}
	return id, true
}

// writeSPCError, SPC hatalarını uygun HTTP yanıtına çevirir.
// Beklenmeyen hataların ayrıntısı log'a yazılır, kullanıcıya gösterilmez.
func writeSPCError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, spc.ErrChartNotFound):
		WriteError(w, http.StatusNotFound, "chart_not_found", "kontrol grafiği bulunamadı")
	case errors.Is(err, spc.ErrChartExists):
		WriteError(w, http.StatusConflict, "chart_exists", "bu adla bir kontrol grafiği zaten var")
	case errors.Is(err, spc.ErrInvalidMeasurement), errors.Is(err, spc.ErrInvalidSubgroups):
		WriteError(w, http.StatusBadRequest, "invalid_measurement", err.Error())
	case errors.Is(err, spc.ErrZeroVariance):
		WriteError(w, http.StatusUnprocessableEntity, "zero_variance", err.Error())
	case errors.Is(err, spc.ErrNotEnoughData):
		WriteError(w, http.StatusConflict, "not_enough_data", err.Error())
	default:
		slog.Error("SPC işlemi başarısız", "error", err)
		WriteError(w, http.StatusInternalServerError, "internal_error", "beklenmeyen bir hata oluştu")
	}
}

func newChartResponse(chart spc.Chart) chartResponse {
	return chartResponse{
		ID:           chart.ID,
		Name:         chart.Name,
		Type:         string(chart.Type),
		SubgroupSize: chart.SubgroupSize,
	}
}

func newLimitsResponse(set spc.LimitSet) *limitsResponse {
	response := &limitsResponse{
		Version: set.Version,
		Center:  set.Mean.Center,
		Sigma:   set.Mean.Sigma,
		UCL:     set.Mean.UCL,
		LCL:     set.Mean.LCL,
	}
	if set.Range != nil {
		response.Range = &rangeLimitsResponse{
			Center: set.Range.Center,
			UCL:    set.Range.UCL,
			LCL:    set.Range.LCL,
		}
	}
	return response
}

func newPointResponses(points []spc.Point) []pointResponse {
	response := make([]pointResponse, 0, len(points))
	for _, p := range points {
		response = append(response, pointResponse{Seq: p.Seq, Values: p.Subgroup, Statistic: p.Statistic})
	}
	return response
}

func newAlarmResponses(alarms []spc.Alarm) []alarmResponse {
	response := make([]alarmResponse, 0, len(alarms))
	for _, a := range alarms {
		response = append(response, alarmResponse{
			PointSeq:     a.PointSeq,
			Rule:         string(a.Rule),
			Value:        a.Value,
			LimitVersion: a.LimitVersion,
		})
	}
	return response
}
