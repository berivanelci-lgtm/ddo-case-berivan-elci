// Package idempotency, aynı Idempotency-Key ile gelen tekrar isteklerin
// ikinci kez işlenmesini önler.
package idempotency

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	MaxKeyLength    = 128
	uniqueViolation = "23505"
)

var (
	// ErrKeyReused, aynı anahtar farklı bir istek gövdesiyle kullanıldığında döner.
	ErrKeyReused = errors.New("bu Idempotency-Key farklı bir istek için kullanılmış")
	// ErrInProgress, aynı anahtarla eşzamanlı bir istek zaten işleniyorsa döner.
	ErrInProgress = errors.New("aynı Idempotency-Key ile bir istek zaten işleniyor")
)

// Stored, daha önce kaydedilmiş bir yanıttır.
type Stored struct {
	StatusCode int
	Body       []byte
}

// Pending, durum değiştiren işlem sırasında kaydedilecek anahtar bilgisidir.
// HTTP katmanı bunu context'e koyar, veritabanı deposu işlem içinde kaydeder.
type Pending struct {
	UserID      int64
	Key         string
	RequestHash string
}

type pendingKey struct{}

// WithPending, anahtar bilgisini context'e koyar.
func WithPending(ctx context.Context, p Pending) context.Context {
	return context.WithValue(ctx, pendingKey{}, p)
}

// PendingFrom, context'teki anahtar bilgisini döner.
func PendingFrom(ctx context.Context) (Pending, bool) {
	p, ok := ctx.Value(pendingKey{}).(Pending)
	return p, ok
}

// Hash, istek gövdesinin SHA-256 özetini döner.
func Hash(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

// Querier, sorgu yöntemleri olan her şeydir (havuz veya işlem).
type Querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Lookup, kullanıcının bu anahtarla daha önce kaydedilmiş bir yanıtı olup olmadığına bakar.
// Yanıt varsa ve istek gövdesi aynıysa onu döner; gövde farklıysa ErrKeyReused döner.
func Lookup(ctx context.Context, q Querier, userID int64, key, requestHash string) (*Stored, error) {
	const query = `
		SELECT request_hash, status_code, response_body
		FROM idempotency_keys
		WHERE user_id = $1 AND key = $2`

	var (
		storedHash string
		stored     Stored
	)
	err := q.QueryRow(ctx, query, userID, key).Scan(&storedHash, &stored.StatusCode, &stored.Body)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("idempotency anahtarı okunamadı: %w", err)
	}
	if storedHash != requestHash {
		return nil, ErrKeyReused
	}
	return &stored, nil
}

// Save, yanıtı anahtarla birlikte kaydeder. Aynı anahtar zaten varsa ErrInProgress döner.
// Çağrı, durum değiştiren işlemin transaction'ı içinde yapılmalıdır; işlem geri alınırsa kayıt da gider.
func Save(ctx context.Context, q Querier, userID int64, key, requestHash string, status int, body []byte) error {
	const query = `
		INSERT INTO idempotency_keys (user_id, key, request_hash, status_code, response_body)
		VALUES ($1, $2, $3, $4, $5)`

	_, err := q.Exec(ctx, query, userID, key, requestHash, status, body)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
		return ErrInProgress
	}
	if err != nil {
		return fmt.Errorf("idempotency anahtarı kaydedilemedi: %w", err)
	}
	return nil
}

// Store, HTTP katmanındaki Idempotent ara katmanının kullandığı PostgreSQL okuyucusudur.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore, yeni bir Store oluşturur.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// Lookup, havuz üzerinden anahtar kaydını arar.
func (s *Store) Lookup(r *http.Request, userID int64, key, requestHash string) (*Stored, error) {
	return Lookup(r.Context(), s.pool, userID, key, requestHash)
}
