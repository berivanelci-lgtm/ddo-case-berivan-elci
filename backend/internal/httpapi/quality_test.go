package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/auth"
	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/quality"
	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/spc"
	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/user"
)

type fakeQualityService struct {
	err       error
	gotForm   quality.Form
	gotValues []float64
	gotUser   int64
}

func (f *fakeQualityService) CreateForm(_ context.Context, form quality.Form, by int64) (quality.Form, error) {
	f.gotForm, f.gotUser = form, by
	form.ID = 4
	return form, f.err
}
func (f *fakeQualityService) ListForms(context.Context) ([]quality.Form, error) { return nil, f.err }
func (f *fakeQualityService) GetForm(_ context.Context, id int64) (quality.Form, error) {
	return quality.Form{ID: id, Code: "QF"}, f.err
}
func (f *fakeQualityService) RecentInspections(context.Context, int64, int) ([]quality.Inspection, error) {
	return nil, f.err
}
func (f *fakeQualityService) Inspect(_ context.Context, formID int64, values []float64, _ *int64, userID int64) (quality.Inspection, []quality.ChartOutcome, error) {
	f.gotValues, f.gotUser = values, userID
	if f.err != nil {
		return quality.Inspection{}, nil, f.err
	}
	return quality.Inspection{ID: 1, FormID: formID, InspectedBy: userID, Values: []quality.Value{
			{Position: 1, Characteristic: "Çap", Value: values[0], InSpec: true},
		}},
		[]quality.ChartOutcome{{Position: 1, ChartID: 7, Seq: 3, Alarms: []spc.Alarm{{PointSeq: 3, Rule: spc.RuleBeyond3Sigma}}}}, nil
}

func qualityCall(t *testing.T, svc *fakeQualityService, role user.Role, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	tokens := newTestTokens(t)
	mux := http.NewServeMux()
	NewQualityAPI(svc).Register(mux, Authenticate(tokens))
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if role != "" {
		req.Header.Set("Authorization", "Bearer "+issueToken(t, tokens, role, auth.AccessToken))
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

const formBody = `{"code":"QF-1","name":"Flanş","gauge_code":"K-1","calibration_due":"2026-12-31",
"items":[{"characteristic":"Çap","unit":"mm","nominal":10,"lsl":9.5,"usl":10.5,"chart_id":7}]}`

func TestQuality_RoleMatrix(t *testing.T) {
	cases := []struct {
		role         user.Role
		method, path string
		body         string
		want         int
	}{
		{"", "GET", "/quality-forms", "", 401},
		{user.RoleKaliteMuhendisi, "POST", "/quality-forms", formBody, 201},
		{user.RoleOperator, "POST", "/quality-forms", formBody, 403},
		{user.RoleOperator, "POST", "/quality-forms/4/inspections", `{"values":[10]}`, 201},
		{user.RoleBakim, "POST", "/quality-forms/4/inspections", `{"values":[10]}`, 403},
		{user.RoleBakim, "GET", "/quality-forms/4", "", 200},
	}
	for _, tc := range cases {
		if rec := qualityCall(t, &fakeQualityService{}, tc.role, tc.method, tc.path, tc.body); rec.Code != tc.want {
			t.Errorf("%q %s %s: %d beklenirdi, gelen %d", tc.role, tc.method, tc.path, tc.want, rec.Code)
		}
	}
}

func TestQuality_CreateFormParsesItemsAndDate(t *testing.T) {
	svc := &fakeQualityService{}
	rec := qualityCall(t, svc, user.RoleKaliteMuhendisi, "POST", "/quality-forms", formBody)

	if rec.Code != 201 || svc.gotUser != 5 || len(svc.gotForm.Items) != 1 ||
		*svc.gotForm.Items[0].ChartID != 7 || svc.gotForm.CalibrationDue.Format("2006-01-02") != "2026-12-31" {
		t.Fatalf("beklenmeyen form: %d %+v", rec.Code, svc.gotForm)
	}
	if !strings.Contains(rec.Body.String(), `"calibration_due":"2026-12-31"`) {
		t.Errorf("yanıtta tarih bulunmalı: %s", rec.Body.String())
	}
}

func TestQuality_CreateFormBadDate(t *testing.T) {
	body := strings.Replace(formBody, "2026-12-31", "31.12.2026", 1)
	if rec := qualityCall(t, &fakeQualityService{}, user.RoleKaliteMuhendisi, "POST", "/quality-forms", body); rec.Code != 400 {
		t.Fatalf("400 beklenirdi, gelen %d", rec.Code)
	}
}

func TestQuality_InspectReturnsSPCOutcome(t *testing.T) {
	svc := &fakeQualityService{}
	rec := qualityCall(t, svc, user.RoleOperator, "POST", "/quality-forms/4/inspections", `{"values":[10.2]}`)

	var resp inspectResponse
	decodeInto(t, rec, &resp)
	if svc.gotValues[0] != 10.2 || resp.FormID != 4 || len(resp.SPC) != 1 || resp.SPC[0].Seq != 3 ||
		resp.SPC[0].Alarms[0].Rule != "beyond_3_sigma" || !resp.Values[0].InSpec {
		t.Fatalf("beklenmeyen yanıt: %+v", resp)
	}
}

func TestQuality_ErrorMapping(t *testing.T) {
	cases := []struct {
		err    error
		status int
		code   string
	}{
		{quality.ErrInvalid, 400, "validation_error"},
		{quality.ErrNotFound, 404, "form_not_found"},
		{quality.ErrCalibrationExpired, 409, "calibration_expired"},
		{errors.New("disk: 10.0.0.18"), 500, "internal_error"},
	}
	for _, tc := range cases {
		rec := qualityCall(t, &fakeQualityService{err: tc.err}, user.RoleOperator, "POST", "/quality-forms/4/inspections", `{"values":[1]}`)
		if rec.Code != tc.status || decodeError(t, rec).Error.Code != tc.code || strings.Contains(rec.Body.String(), "10.0.0.18") {
			t.Errorf("%v: %d/%s beklenirdi, gelen %d %s", tc.err, tc.status, tc.code, rec.Code, rec.Body.String())
		}
	}
}
