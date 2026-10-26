// Package metrics, Prometheus metin biçiminde basit HTTP metrikleri üretir.
package metrics

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"
)

// DefaultBuckets, istek süresi histogramının üst sınırlarıdır (saniye).
var DefaultBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5}

type requestKey struct {
	method string
	status int
}

// Registry, istek sayılarını ve süre histogramını tutar. Eşzamanlı kullanım için güvenlidir.
type Registry struct {
	mu       sync.Mutex
	requests map[requestKey]uint64
	buckets  []float64
	counts   []uint64 // her kova için (kümülatif olmayan) sayı
	sum      float64
	total    uint64
	started  time.Time
	now      func() time.Time
}

// New, yeni bir Registry oluşturur.
func New() *Registry {
	return &Registry{
		requests: make(map[requestKey]uint64),
		buckets:  DefaultBuckets,
		counts:   make([]uint64, len(DefaultBuckets)),
		started:  time.Now(),
		now:      time.Now,
	}
}

// Observe, tamamlanan bir isteği kaydeder.
func (r *Registry) Observe(method string, status int, duration time.Duration) {
	seconds := duration.Seconds()

	r.mu.Lock()
	defer r.mu.Unlock()

	r.requests[requestKey{method: method, status: status}]++
	r.sum += seconds
	r.total++
	for i, upper := range r.buckets {
		if seconds <= upper {
			r.counts[i]++
			break
		}
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(status int) {
	s.status = status
	s.ResponseWriter.WriteHeader(status)
}

// Middleware, her isteğin yöntemini, durum kodunu ve süresini kaydeder.
func (r *Registry) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		start := r.now()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(recorder, req)
		r.Observe(req.Method, recorder.status, r.now().Sub(start))
	})
}

// Handler, metrikleri Prometheus metin biçiminde yazar.
func (r *Registry) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		_, _ = w.Write([]byte(r.Render()))
	}
}

// Render, metriklerin metin gösterimini döner.
func (r *Registry) Render() string {
	r.mu.Lock()
	defer r.mu.Unlock()

	keys := make([]requestKey, 0, len(r.requests))
	for key := range r.requests {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].method != keys[j].method {
			return keys[i].method < keys[j].method
		}
		return keys[i].status < keys[j].status
	})

	var out []byte
	out = append(out, "# HELP hatkontrol_http_requests_total Total HTTP requests by method and status.\n"...)
	out = append(out, "# TYPE hatkontrol_http_requests_total counter\n"...)
	for _, key := range keys {
		out = fmt.Appendf(out, "hatkontrol_http_requests_total{method=%q,status=\"%d\"} %d\n",
			key.method, key.status, r.requests[key])
	}

	out = append(out, "# HELP hatkontrol_http_request_duration_seconds HTTP request duration.\n"...)
	out = append(out, "# TYPE hatkontrol_http_request_duration_seconds histogram\n"...)
	var cumulative uint64
	for i, upper := range r.buckets {
		cumulative += r.counts[i]
		out = fmt.Appendf(out, "hatkontrol_http_request_duration_seconds_bucket{le=\"%s\"} %d\n",
			strconv.FormatFloat(upper, 'g', -1, 64), cumulative)
	}
	out = fmt.Appendf(out, "hatkontrol_http_request_duration_seconds_bucket{le=\"+Inf\"} %d\n", r.total)
	out = fmt.Appendf(out, "hatkontrol_http_request_duration_seconds_sum %s\n", strconv.FormatFloat(r.sum, 'f', -1, 64))
	out = fmt.Appendf(out, "hatkontrol_http_request_duration_seconds_count %d\n", r.total)

	out = append(out, "# HELP hatkontrol_uptime_seconds Seconds since the process started.\n"...)
	out = append(out, "# TYPE hatkontrol_uptime_seconds gauge\n"...)
	out = fmt.Appendf(out, "hatkontrol_uptime_seconds %d\n", int64(r.now().Sub(r.started).Seconds()))
	return string(out)
}
