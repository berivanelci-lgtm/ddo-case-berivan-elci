package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/sensor"
	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/user"
)

// SensorIngester, sensör okumalarını yazan her şeydir.
type SensorIngester interface {
	Ingest(ctx context.Context, readings []sensor.Reading) (int64, error)
}

// SensorsAPI, toplu sensör girdisi ucunu sunar.
type SensorsAPI struct {
	service SensorIngester
}

// NewSensorsAPI, yeni bir SensorsAPI oluşturur.
func NewSensorsAPI(service SensorIngester) *SensorsAPI {
	return &SensorsAPI{service: service}
}

// Register, ucu ekler. İstasyon verisini operatör, hat şefi ve bakım gönderebilir.
func (a *SensorsAPI) Register(mux *http.ServeMux, authenticate func(http.Handler) http.Handler) {
	mux.Handle("POST /sensor-readings", protected(authenticate, a.ingest,
		user.RoleOperator, user.RoleHatSefi, user.RoleBakim))
}

type sensorReadingRequest struct {
	StationID  int32     `json:"station_id"`
	RecordedAt time.Time `json:"recorded_at"`
	Value      float64   `json:"value"`
}

type sensorBatchRequest struct {
	Readings []sensorReadingRequest `json:"readings"`
}

type ingestResponse struct {
	Inserted int64 `json:"inserted"`
}

func (a *SensorsAPI) ingest(w http.ResponseWriter, r *http.Request) {
	var req sensorBatchRequest
	if !decodeBody(w, r, &req) {
		return
	}

	readings := make([]sensor.Reading, 0, len(req.Readings))
	for _, item := range req.Readings {
		readings = append(readings, sensor.Reading{StationID: item.StationID, RecordedAt: item.RecordedAt, Value: item.Value})
	}

	n, err := a.service.Ingest(r.Context(), readings)
	switch {
	case errors.Is(err, sensor.ErrInvalid):
		WriteError(w, http.StatusBadRequest, "validation_error", err.Error())
	case err != nil:
		slog.Error("sensör verisi yazılamadı", "error", err)
		WriteError(w, http.StatusInternalServerError, "internal_error", "beklenmeyen bir hata oluştu")
	default:
		WriteJSON(w, http.StatusCreated, ingestResponse{Inserted: n})
	}
}
