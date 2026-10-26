package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// documentedRoutes, main.go'da kayıtlı tüm yollardır. Yeni bir uç eklendiğinde buraya ve
// openapi.yaml'a eklenmelidir; test, ikisinin uyumsuz kalmasını yakalar.
var documentedRoutes = []string{
	"/health", "/metrics", "/openapi.yaml",
	"/auth/login", "/auth/refresh", "/me",
	"/charts", "/charts/{id}", "/charts/{id}/measurements", "/charts/{id}/limits/recompute",
	"/materials", "/products",
	"/work-orders", "/work-orders/{id}", "/work-orders/{id}/release", "/work-orders/{id}/cancel",
	"/work-orders/{id}/production", "/work-orders/{id}/rework", "/work-orders/{id}/close",
	"/audit-log",
	"/traceability/lots/{code}/backward", "/traceability/lots/{code}/forward", "/traceability/lots/{code}/recall",
	"/ncrs", "/ncrs/{id}", "/ncrs/{id}/investigate", "/ncrs/{id}/close",
	"/lines", "/downtime-reasons", "/lines/{code}/downtime", "/lines/{code}/counts", "/lines/{code}/oee",
	"/sensor-readings",
	"/quality-forms", "/quality-forms/{id}", "/quality-forms/{id}/inspections",
}

func TestOpenAPI_DocumentsEveryRoute(t *testing.T) {
	spec := strings.ReplaceAll(string(openAPISpec), "\r\n", "\n")
	for _, route := range documentedRoutes {
		if !strings.Contains(spec, "\n  "+route+":\n") {
			t.Errorf("openapi.yaml içinde %s yolu yok", route)
		}
	}
}

func TestOpenAPI_IsServed(t *testing.T) {
	rec := httptest.NewRecorder()
	OpenAPIHandler()(rec, httptest.NewRequest(http.MethodGet, "/openapi.yaml", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("200 beklenirdi, gelen %d", rec.Code)
	}
	if !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/yaml") {
		t.Errorf("application/yaml beklenirdi, gelen %q", rec.Header().Get("Content-Type"))
	}
	if !strings.HasPrefix(rec.Body.String(), "openapi: 3.0.3") {
		t.Error("doküman openapi: 3.0.3 ile başlamalı")
	}
}
