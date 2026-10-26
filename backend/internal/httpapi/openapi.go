package httpapi

import (
	_ "embed"
	"net/http"
)

// openAPISpec, API'nin OpenAPI 3 dokümanıdır; programın içine gömülür.
//
//go:embed openapi.yaml
var openAPISpec []byte

// OpenAPIHandler, OpenAPI dokümanını sunar.
func OpenAPIHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
		_, _ = w.Write(openAPISpec)
	}
}
