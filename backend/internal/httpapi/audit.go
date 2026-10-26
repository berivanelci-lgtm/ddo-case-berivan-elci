package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/audit"
	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/user"
)

// auditEntityTypes, denetim kaydı tutulan tabloların adlarıdır.
var auditEntityTypes = map[string]bool{
	"charts":         true,
	"materials":      true,
	"products":       true,
	"work_orders":    true,
	"spc_limit_sets": true,
	"ncrs":           true,
}

// AuditReader, denetim kayıtlarını okuyan her şeyi temsil eder.
type AuditReader interface {
	List(ctx context.Context, entityType string, before int64, limit int) ([]audit.Record, error)
}

// AuditAPI, denetim kaydı okuma ucunu sunar.
type AuditAPI struct {
	reader AuditReader
}

// NewAuditAPI, yeni bir AuditAPI oluşturur.
func NewAuditAPI(reader AuditReader) *AuditAPI {
	return &AuditAPI{reader: reader}
}

// Register, denetim ucunu mux'a ekler. Yalnızca fabrika müdürü okuyabilir.
func (a *AuditAPI) Register(mux *http.ServeMux, authenticate func(http.Handler) http.Handler) {
	mux.Handle("GET /audit-log", protected(authenticate, a.list, user.RoleFabrikaMuduru))
}

type auditEntryResponse struct {
	ID         int64           `json:"id"`
	OccurredAt time.Time       `json:"occurred_at"`
	UserID     *int64          `json:"user_id"`
	Action     string          `json:"action"`
	EntityType string          `json:"entity_type"`
	EntityID   int64           `json:"entity_id"`
	OldValue   json.RawMessage `json:"old_value"`
	NewValue   json.RawMessage `json:"new_value"`
}

type auditListResponse struct {
	Items      []auditEntryResponse `json:"items"`
	NextBefore *int64               `json:"next_before"`
}

func (a *AuditAPI) list(w http.ResponseWriter, r *http.Request) {
	limit, before, ok := parsePaging(w, r)
	if !ok {
		return
	}

	entityType := r.URL.Query().Get("entity_type")
	if entityType != "" && !auditEntityTypes[entityType] {
		WriteError(w, http.StatusBadRequest, "invalid_entity_type",
			"entity_type şunlardan biri olmalı: charts, materials, products, work_orders, spc_limit_sets, ncrs")
		return
	}

	// Bir fazla kayıt isteyerek sonraki sayfa olup olmadığını anlarız.
	records, err := a.reader.List(r.Context(), entityType, before, limit+1)
	if err != nil {
		slog.Error("denetim kayıtları okunamadı", "error", err)
		WriteError(w, http.StatusInternalServerError, "internal_error", "beklenmeyen bir hata oluştu")
		return
	}

	response := auditListResponse{Items: make([]auditEntryResponse, 0, len(records))}
	if len(records) > limit {
		records = records[:limit]
		next := records[len(records)-1].ID
		response.NextBefore = &next
	}
	for _, record := range records {
		response.Items = append(response.Items, auditEntryResponse{
			ID:         record.ID,
			OccurredAt: record.OccurredAt,
			UserID:     record.UserID,
			Action:     record.Action,
			EntityType: record.EntityType,
			EntityID:   record.EntityID,
			OldValue:   record.Old,
			NewValue:   record.New,
		})
	}
	WriteJSON(w, http.StatusOK, response)
}
