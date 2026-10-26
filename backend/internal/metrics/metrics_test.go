package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMiddleware_CountsByMethodAndStatus(t *testing.T) {
	registry := New()
	handler := registry.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/missing" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))

	for _, path := range []string{"/a", "/a", "/missing"} {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))
	}

	out := registry.Render()
	for _, want := range []string{
		`hatkontrol_http_requests_total{method="GET",status="200"} 2`,
		`hatkontrol_http_requests_total{method="GET",status="404"} 1`,
		`hatkontrol_http_request_duration_seconds_count 3`,
		`hatkontrol_http_request_duration_seconds_bucket{le="+Inf"} 3`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("çıktıda %q bulunmalı:\n%s", want, out)
		}
	}
}

func TestObserve_HistogramIsCumulative(t *testing.T) {
	registry := New()
	registry.Observe("GET", 200, 3*time.Millisecond)  // 0.005 kovası
	registry.Observe("GET", 200, 30*time.Millisecond) // 0.05 kovası
	registry.Observe("GET", 200, 10*time.Second)      // yalnızca +Inf

	out := registry.Render()
	for _, want := range []string{
		`_bucket{le="0.005"} 1`,
		`_bucket{le="0.025"} 1`,
		`_bucket{le="0.05"} 2`,
		`_bucket{le="5"} 2`,
		`_bucket{le="+Inf"} 3`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("çıktıda %q bulunmalı:\n%s", want, out)
		}
	}
}

func TestHandler_ContentType(t *testing.T) {
	rec := httptest.NewRecorder()
	New().Handler()(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	if !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/plain") {
		t.Errorf("text/plain beklenirdi, gelen %q", rec.Header().Get("Content-Type"))
	}
	if !strings.Contains(rec.Body.String(), "# TYPE hatkontrol_uptime_seconds gauge") {
		t.Error("uptime metriği bulunmalı")
	}
}
