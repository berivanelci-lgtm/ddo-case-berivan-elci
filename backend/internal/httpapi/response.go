// Package httpapi, HTTP uçlarını ve ortak yanıt yardımcılarını içerir.
package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// errorBody, tüm hata yanıtlarının ortak biçimidir.
type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// WriteJSON, verilen değeri JSON olarak yazar.
func WriteJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		slog.Error("yanıt yazılamadı", "error", err)
	}
}

// WriteError, tutarlı biçimde bir hata yanıtı yazar.
func WriteError(w http.ResponseWriter, status int, code, message string) {
	WriteJSON(w, status, errorBody{Error: errorDetail{Code: code, Message: message}})
}
