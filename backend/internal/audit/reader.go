package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Record, denetim tablosundaki bir kaydı temsil eder.
type Record struct {
	ID         int64
	OccurredAt time.Time
	UserID     *int64 // boşsa işlem API dışında, veritabanında doğrudan yapılmıştır
	Action     string // insert, update ya da delete
	EntityType string // tablo adı
	EntityID   int64
	Old        json.RawMessage
	New        json.RawMessage
}

// Reader, denetim kayıtlarını okur.
type Reader struct {
	pool *pgxpool.Pool
}

// NewReader, yeni bir Reader oluşturur.
func NewReader(pool *pgxpool.Pool) *Reader {
	return &Reader{pool: pool}
}

// List, kayıtları yeniden eskiye sıralı döner (keyset sayfalama).
// entityType boş değilse yalnızca o tablonun kayıtları, before sıfırdan büyükse
// yalnızca kimliği before'dan küçük olanlar gelir.
func (r *Reader) List(ctx context.Context, entityType string, before int64, limit int) ([]Record, error) {
	const query = `
		SELECT id, occurred_at, user_id, action, entity_type, entity_id, old_value, new_value
		FROM audit_log
		WHERE ($1::text = '' OR entity_type = $1::text)
		  AND ($2::bigint = 0 OR id < $2::bigint)
		ORDER BY id DESC
		LIMIT $3`

	rows, err := r.pool.Query(ctx, query, entityType, before, limit)
	if err != nil {
		return nil, fmt.Errorf("denetim kayıtları sorgulanamadı: %w", err)
	}
	defer rows.Close()

	var records []Record
	for rows.Next() {
		var (
			record         Record
			oldRaw, newRaw []byte
		)
		err := rows.Scan(&record.ID, &record.OccurredAt, &record.UserID, &record.Action,
			&record.EntityType, &record.EntityID, &oldRaw, &newRaw)
		if err != nil {
			return nil, fmt.Errorf("denetim kaydı okunamadı: %w", err)
		}
		record.Old = json.RawMessage(oldRaw)
		record.New = json.RawMessage(newRaw)
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("denetim kayıtları okunamadı: %w", err)
	}
	return records, nil
}
