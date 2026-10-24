package httpapi

import (
	"context"
	"net/http"
	"time"
)

const healthCheckTimeout = 2 * time.Second

// Pinger, veritabanına ulaşılabilirliği kontrol eden her şeyi temsil eder.
type Pinger interface {
	Ping(ctx context.Context) error
}

type healthResponse struct {
	Status   string `json:"status"`
	Database string `json:"database"`
}

// HealthHandler, servisin ve veritabanının sağlık durumunu döner.
func HealthHandler(db Pinger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), healthCheckTimeout)
		defer cancel()

		if err := db.Ping(ctx); err != nil {
			WriteError(w, http.StatusServiceUnavailable, "database_unavailable", "veritabanına ulaşılamıyor")
			return
		}

		WriteJSON(w, http.StatusOK, healthResponse{Status: "ok", Database: "ok"})
	}
}
