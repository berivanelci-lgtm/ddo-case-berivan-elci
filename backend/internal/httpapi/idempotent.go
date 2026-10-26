package httpapi

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/idempotency"
)

const idempotencyHeader = "Idempotency-Key"

// IdempotencyStore, anahtar kaydını okuyabilen her şeydir.
type IdempotencyStore interface {
	Lookup(r *http.Request, userID int64, key, requestHash string) (*idempotency.Stored, error)
}

// Idempotent, Idempotency-Key başlığı olan istekleri ikinci kez işlemez.
// Anahtar yoksa istek olduğu gibi işlenir.
// Daha önce başarıyla işlenmiş bir anahtar gelirse kayıtlı yanıt döner (Idempotent-Replayed: true).
// Yeni anahtar için bilgi context'e konur; kayıt işlemin transaction'ı içinde yazılır.
func Idempotent(store IdempotencyStore, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get(idempotencyHeader)
		if key == "" {
			next(w, r)
			return
		}
		if len(key) > idempotency.MaxKeyLength {
			WriteError(w, http.StatusBadRequest, "invalid_idempotency_key", "Idempotency-Key en fazla 128 karakter olmalı")
			return
		}

		claims, ok := ClaimsFrom(r.Context())
		if !ok {
			writeUnauthorized(w, "missing_token", "kimlik doğrulama gerekli")
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_request", "istek gövdesi okunamadı")
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))

		hash := idempotency.Hash(body)
		stored, err := store.Lookup(r, claims.UserID, key, hash)
		if errors.Is(err, idempotency.ErrKeyReused) {
			WriteError(w, http.StatusUnprocessableEntity, "idempotency_key_reused", err.Error())
			return
		}
		if err != nil {
			slog.Error("idempotency sorgusu başarısız", "error", err)
			WriteError(w, http.StatusInternalServerError, "internal_error", "beklenmeyen bir hata oluştu")
			return
		}
		if stored != nil {
			w.Header().Set("Idempotent-Replayed", "true")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(stored.StatusCode)
			_, _ = w.Write(stored.Body)
			return
		}

		ctx := idempotency.WithPending(r.Context(), idempotency.Pending{
			UserID:      claims.UserID,
			Key:         key,
			RequestHash: hash,
		})
		next(w, r.WithContext(ctx))
	}
}
