package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/auth"
	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/spc"
	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/user"
)

// --- Sahte servis ve depo ---

type fakeChartService struct {
	result spc.Result
	set    spc.LimitSet
	err    error

	gotChartID int64
	gotValues  []float64
}

func (f *fakeChartService) AddMeasurement(_ context.Context, chartID int64, subgroup []float64) (spc.Result, error) {
	f.gotChartID, f.gotValues = chartID, subgroup
	return f.result, f.err
}

func (f *fakeChartService) RecomputeLimits(_ context.Context, chartID int64) (spc.LimitSet, error) {
	f.gotChartID = chartID
	return f.set, f.err
}

type fakeChartRepo struct {
	charts    []spc.Chart
	limits    *spc.LimitSet
	points    []spc.Point
	alarms    []spc.Alarm
	createErr error

	createdName string
	createdType spc.ChartType
	createdSize int
	createdBy   int64
}

func (f *fakeChartRepo) CreateChart(_ context.Context, name string, chartType spc.ChartType, size int, createdBy int64) (spc.Chart, error) {
	if f.createErr != nil {
		return spc.Chart{}, f.createErr
	}
	f.createdName, f.createdType, f.createdSize, f.createdBy = name, chartType, size, createdBy
	return spc.Chart{ID: 10, Name: name, Type: chartType, SubgroupSize: size}, nil
}

func (f *fakeChartRepo) GetChart(_ context.Context, id int64) (spc.Chart, error) {
	for _, chart := range f.charts {
		if chart.ID == id {
			return chart, nil
		}
	}
	return spc.Chart{}, spc.ErrChartNotFound
}

func (f *fakeChartRepo) ListCharts(context.Context) ([]spc.Chart, error) { return f.charts, nil }

func (f *fakeChartRepo) LatestLimits(context.Context, int64) (*spc.LimitSet, error) {
	return f.limits, nil
}

func (f *fakeChartRepo) RecentPoints(context.Context, int64, int) ([]spc.Point, error) {
	return f.points, nil
}

func (f *fakeChartRepo) RecentAlarms(context.Context, int64, int) ([]spc.Alarm, error) {
	return f.alarms, nil
}

// --- Test ortamı ---

type chartsTestEnv struct {
	handler http.Handler
	tokens  *auth.TokenManager
	service *fakeChartService
	repo    *fakeChartRepo
}

func newChartsEnv(t *testing.T) *chartsTestEnv {
	t.Helper()
	tokens := newTestTokens(t)
	service := &fakeChartService{}
	repo := &fakeChartRepo{charts: []spc.Chart{
		{ID: 1, Name: "çap", Type: spc.ChartIndividual, SubgroupSize: 1},
	}}

	mux := http.NewServeMux()
	NewChartsAPI(service, repo).Register(mux, Authenticate(tokens))

	return &chartsTestEnv{handler: mux, tokens: tokens, service: service, repo: repo}
}

// call, verilen rolün token'ıyla bir istek gönderir. Rol boşsa token eklenmez.
func (e *chartsTestEnv) call(t *testing.T, role user.Role, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if role != "" {
		req.Header.Set("Authorization", "Bearer "+issueToken(t, e.tokens, role, auth.AccessToken))
	}
	rec := httptest.NewRecorder()
	e.handler.ServeHTTP(rec, req)
	return rec
}

func decodeInto(t *testing.T, rec *httptest.ResponseRecorder, dst any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), dst); err != nil {
		t.Fatalf("yanıt çözülemedi: %v (gövde: %s)", err, rec.Body.String())
	}
}

// --- Kimlik ve yetki testleri ---

func TestCharts_RequireAuthentication(t *testing.T) {
	endpoints := []struct{ name, method, path string }{
		{"grafik_olustur", http.MethodPost, "/charts"},
		{"grafik_listele", http.MethodGet, "/charts"},
		{"grafik_detay", http.MethodGet, "/charts/1"},
		{"olcum_ekle", http.MethodPost, "/charts/1/measurements"},
		{"limit_yenile", http.MethodPost, "/charts/1/limits/recompute"},
	}

	for _, endpoint := range endpoints {
		t.Run(endpoint.name, func(t *testing.T) {
			rec := newChartsEnv(t).call(t, "", endpoint.method, endpoint.path, "")

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("401 beklenirdi, gelen %d", rec.Code)
			}
		})
	}
}

func TestCharts_RoleMatrix(t *testing.T) {
	const (
		createBody  = `{"name":"yeni grafik","type":"individual"}`
		measureBody = `{"values":[10.2]}`
	)

	cases := []struct {
		name         string
		role         user.Role
		method, path string
		body         string
		want         int
	}{
		{"operatör grafik oluşturamaz", user.RoleOperator, "POST", "/charts", createBody, 403},
		{"bakım grafik oluşturamaz", user.RoleBakim, "POST", "/charts", createBody, 403},
		{"planlama grafik oluşturamaz", user.RoleUretimPlanlama, "POST", "/charts", createBody, 403},
		{"kalite grafik oluşturur", user.RoleKaliteMuhendisi, "POST", "/charts", createBody, 201},
		{"hat şefi grafik oluşturur", user.RoleHatSefi, "POST", "/charts", createBody, 201},
		{"müdür grafik oluşturur", user.RoleFabrikaMuduru, "POST", "/charts", createBody, 201},

		{"bakım ölçüm giremez", user.RoleBakim, "POST", "/charts/1/measurements", measureBody, 403},
		{"planlama ölçüm giremez", user.RoleUretimPlanlama, "POST", "/charts/1/measurements", measureBody, 403},
		{"müdür ölçüm giremez", user.RoleFabrikaMuduru, "POST", "/charts/1/measurements", measureBody, 403},
		{"operatör ölçüm girer", user.RoleOperator, "POST", "/charts/1/measurements", measureBody, 201},
		{"hat şefi ölçüm girer", user.RoleHatSefi, "POST", "/charts/1/measurements", measureBody, 201},
		{"kalite ölçüm girer", user.RoleKaliteMuhendisi, "POST", "/charts/1/measurements", measureBody, 201},

		{"operatör limit yenileyemez", user.RoleOperator, "POST", "/charts/1/limits/recompute", "", 403},
		{"hat şefi limit yenileyemez", user.RoleHatSefi, "POST", "/charts/1/limits/recompute", "", 403},
		{"kalite limit yeniler", user.RoleKaliteMuhendisi, "POST", "/charts/1/limits/recompute", "", 201},

		{"bakım grafikleri listeler", user.RoleBakim, "GET", "/charts", "", 200},
		{"operatör grafik detayını görür", user.RoleOperator, "GET", "/charts/1", "", 200},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := newChartsEnv(t).call(t, tc.role, tc.method, tc.path, tc.body)

			if rec.Code != tc.want {
				t.Fatalf("%d beklenirdi, gelen %d (gövde: %s)", tc.want, rec.Code, rec.Body.String())
			}
		})
	}
}

// --- Grafik oluşturma ---

func TestCreateChart_Success(t *testing.T) {
	env := newChartsEnv(t)

	rec := env.call(t, user.RoleKaliteMuhendisi, "POST", "/charts", `{"name":"  Çap  ","type":"individual"}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("201 beklenirdi, gelen %d", rec.Code)
	}
	var resp chartResponse
	decodeInto(t, rec, &resp)
	if resp.ID != 10 || resp.Name != "Çap" || resp.Type != "individual" || resp.SubgroupSize != 1 {
		t.Errorf("beklenmeyen yanıt: %+v", resp)
	}
	if env.repo.createdBy != 5 {
		t.Errorf("oluşturan kullanıcı token'daki kullanıcı (5) olmalı, gelen %d", env.repo.createdBy)
	}
}

func TestCreateChart_Validation(t *testing.T) {
	cases := map[string]string{
		"boş ad":           `{"name":"","type":"individual"}`,
		"boşluktan oluşan": `{"name":"   ","type":"individual"}`,
		"çok uzun ad":      `{"name":"` + strings.Repeat("a", 101) + `","type":"individual"}`,
		"bilinmeyen tür":   `{"name":"x","type":"pie"}`,
		"tür yok":          `{"name":"x"}`,
		"bireysel için 3":  `{"name":"x","type":"individual","subgroup_size":3}`,
		"xbar için 1":      `{"name":"x","type":"xbar_r","subgroup_size":1}`,
		"xbar için 11":     `{"name":"x","type":"xbar_r","subgroup_size":11}`,
		"xbar için eksik":  `{"name":"x","type":"xbar_r"}`,
	}

	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			env := newChartsEnv(t)

			rec := env.call(t, user.RoleKaliteMuhendisi, "POST", "/charts", body)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("400 beklenirdi, gelen %d", rec.Code)
			}
			if code := decodeError(t, rec).Error.Code; code != "validation_error" {
				t.Errorf("validation_error beklenirdi, gelen %q", code)
			}
			if env.repo.createdName != "" {
				t.Error("geçersiz istekle grafik oluşturulmamalı")
			}
		})
	}
}

func TestCreateChart_DuplicateName(t *testing.T) {
	env := newChartsEnv(t)
	env.repo.createErr = spc.ErrChartExists

	rec := env.call(t, user.RoleKaliteMuhendisi, "POST", "/charts", `{"name":"çap","type":"individual"}`)

	if rec.Code != http.StatusConflict {
		t.Fatalf("409 beklenirdi, gelen %d", rec.Code)
	}
	if code := decodeError(t, rec).Error.Code; code != "chart_exists" {
		t.Errorf("chart_exists beklenirdi, gelen %q", code)
	}
}

func TestCreateChart_RejectsUnknownFields(t *testing.T) {
	rec := newChartsEnv(t).call(t, user.RoleKaliteMuhendisi, "POST", "/charts",
		`{"name":"x","type":"individual","created_by":1}`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("400 beklenirdi, gelen %d", rec.Code)
	}
	if code := decodeError(t, rec).Error.Code; code != "invalid_request" {
		t.Errorf("invalid_request beklenirdi, gelen %q", code)
	}
}

// --- Ölçüm ekleme ---

func TestAddMeasurement_Success(t *testing.T) {
	env := newChartsEnv(t)
	env.service.result = spc.Result{
		Seq:          26,
		Statistic:    16,
		LimitVersion: 1,
		Alarms: []spc.Alarm{
			{ChartID: 1, PointSeq: 26, Rule: spc.RuleBeyond3Sigma, Value: 16, LimitVersion: 1},
		},
	}

	rec := env.call(t, user.RoleOperator, "POST", "/charts/1/measurements", `{"values":[16]}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("201 beklenirdi, gelen %d", rec.Code)
	}
	var resp measurementResponse
	decodeInto(t, rec, &resp)
	if resp.Seq != 26 || resp.Statistic != 16 || resp.LimitVersion != 1 || resp.Limits != nil {
		t.Errorf("beklenmeyen yanıt: %+v", resp)
	}
	if len(resp.Alarms) != 1 || resp.Alarms[0].Rule != "beyond_3_sigma" || resp.Alarms[0].PointSeq != 26 {
		t.Errorf("beklenmeyen alarmlar: %+v", resp.Alarms)
	}
	if env.service.gotChartID != 1 || len(env.service.gotValues) != 1 || env.service.gotValues[0] != 16 {
		t.Errorf("servise yanlış değerler gitti: grafik %d, değerler %v", env.service.gotChartID, env.service.gotValues)
	}
}

func TestAddMeasurement_InvalidChartID(t *testing.T) {
	for _, path := range []string{"/charts/abc/measurements", "/charts/0/measurements", "/charts/-1/measurements"} {
		t.Run(path, func(t *testing.T) {
			env := newChartsEnv(t)

			rec := env.call(t, user.RoleOperator, "POST", path, `{"values":[1]}`)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("400 beklenirdi, gelen %d", rec.Code)
			}
			if code := decodeError(t, rec).Error.Code; code != "invalid_id" {
				t.Errorf("invalid_id beklenirdi, gelen %q", code)
			}
			if env.service.gotChartID != 0 {
				t.Error("geçersiz kimlikle servis çağrılmamalı")
			}
		})
	}
}

func TestAddMeasurement_ErrorMapping(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"grafik yok", spc.ErrChartNotFound, 404, "chart_not_found"},
		{"geçersiz ölçüm", fmt.Errorf("%w: 2 değer beklenirdi", spc.ErrInvalidMeasurement), 400, "invalid_measurement"},
		{"sıfır değişim", spc.ErrZeroVariance, 422, "zero_variance"},
		{"beklenmeyen hata", errors.New("bağlantı koptu: 10.0.0.9"), 500, "internal_error"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newChartsEnv(t)
			env.service.err = tc.err

			rec := env.call(t, user.RoleOperator, "POST", "/charts/1/measurements", `{"values":[1]}`)

			if rec.Code != tc.status {
				t.Fatalf("%d beklenirdi, gelen %d", tc.status, rec.Code)
			}
			if code := decodeError(t, rec).Error.Code; code != tc.code {
				t.Errorf("%s beklenirdi, gelen %q", tc.code, code)
			}
			if tc.status == http.StatusInternalServerError && strings.Contains(rec.Body.String(), "10.0.0.9") {
				t.Error("iç hata ayrıntısı kullanıcıya sızmamalı")
			}
		})
	}
}

func TestAddMeasurement_MalformedBody(t *testing.T) {
	rec := newChartsEnv(t).call(t, user.RoleOperator, "POST", "/charts/1/measurements", `{bozuk`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("400 beklenirdi, gelen %d", rec.Code)
	}
	if code := decodeError(t, rec).Error.Code; code != "invalid_request" {
		t.Errorf("invalid_request beklenirdi, gelen %q", code)
	}
}

// --- Grafik okuma ---

func TestGetChart_ReturnsChartLimitsPointsAndAlarms(t *testing.T) {
	env := newChartsEnv(t)
	env.repo.limits = &spc.LimitSet{Version: 1, Mean: spc.Limits{Center: 10, Sigma: 1, UCL: 13, LCL: 7}}
	env.repo.points = []spc.Point{{Seq: 1, Subgroup: []float64{10}, Statistic: 10}}
	env.repo.alarms = []spc.Alarm{{ChartID: 1, PointSeq: 1, Rule: spc.RuleTrend6, Value: 10, LimitVersion: 1}}

	rec := env.call(t, user.RoleBakim, "GET", "/charts/1", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("200 beklenirdi, gelen %d", rec.Code)
	}
	var resp chartDetailResponse
	decodeInto(t, rec, &resp)
	if resp.Chart.Name != "çap" {
		t.Errorf("grafik adı çap olmalı, gelen %q", resp.Chart.Name)
	}
	if resp.Limits == nil || resp.Limits.UCL != 13 || resp.Limits.Range != nil {
		t.Errorf("beklenmeyen limitler: %+v", resp.Limits)
	}
	if len(resp.Points) != 1 || len(resp.Points[0].Values) != 1 {
		t.Errorf("beklenmeyen noktalar: %+v", resp.Points)
	}
	if len(resp.Alarms) != 1 || resp.Alarms[0].Rule != "trend_6" {
		t.Errorf("beklenmeyen alarmlar: %+v", resp.Alarms)
	}
}

func TestGetChart_NotFound(t *testing.T) {
	rec := newChartsEnv(t).call(t, user.RoleBakim, "GET", "/charts/99", "")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("404 beklenirdi, gelen %d", rec.Code)
	}
	if code := decodeError(t, rec).Error.Code; code != "chart_not_found" {
		t.Errorf("chart_not_found beklenirdi, gelen %q", code)
	}
}

func TestGetChart_EmptyCollectionsAreArrays(t *testing.T) {
	rec := newChartsEnv(t).call(t, user.RoleBakim, "GET", "/charts/1", "")

	body := rec.Body.String()
	for _, want := range []string{`"points":[]`, `"alarms":[]`, `"limits":null`} {
		if !strings.Contains(body, want) {
			t.Errorf("yanıtta %s bulunmalı, gövde: %s", want, body)
		}
	}
}

func TestListCharts_EmptyIsArray(t *testing.T) {
	env := newChartsEnv(t)
	env.repo.charts = nil

	rec := env.call(t, user.RoleBakim, "GET", "/charts", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("200 beklenirdi, gelen %d", rec.Code)
	}
	if body := strings.TrimSpace(rec.Body.String()); body != "[]" {
		t.Errorf("boş liste [] olmalı, gelen %s", body)
	}
}

// --- Limit yenileme ---

func TestRecomputeLimits_Success(t *testing.T) {
	env := newChartsEnv(t)
	env.service.set = spc.LimitSet{Version: 2, Mean: spc.Limits{Center: 10, Sigma: 1, UCL: 13, LCL: 7}}

	rec := env.call(t, user.RoleKaliteMuhendisi, "POST", "/charts/1/limits/recompute", "")

	if rec.Code != http.StatusCreated {
		t.Fatalf("201 beklenirdi, gelen %d", rec.Code)
	}
	var resp limitsResponse
	decodeInto(t, rec, &resp)
	if resp.Version != 2 || resp.UCL != 13 {
		t.Errorf("beklenmeyen yanıt: %+v", resp)
	}
}

func TestRecomputeLimits_NotEnoughData(t *testing.T) {
	env := newChartsEnv(t)
	env.service.err = fmt.Errorf("%w: ilk limitler henüz hesaplanmadı", spc.ErrNotEnoughData)

	rec := env.call(t, user.RoleKaliteMuhendisi, "POST", "/charts/1/limits/recompute", "")

	if rec.Code != http.StatusConflict {
		t.Fatalf("409 beklenirdi, gelen %d", rec.Code)
	}
	if code := decodeError(t, rec).Error.Code; code != "not_enough_data" {
		t.Errorf("not_enough_data beklenirdi, gelen %q", code)
	}
}
