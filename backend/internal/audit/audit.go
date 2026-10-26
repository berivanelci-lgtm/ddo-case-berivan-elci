// Package audit, denetim kaydı için işlemi yapan kullanıcıyı veritabanı oturumuna taşır
// ve kayıtları okur. Kayıtların kendisi veritabanı tetikleyicileriyle (migrasyon 00004) yazılır.
package audit

import (
	"context"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type userKey struct{}

// WithUser, işlemi yapan kullanıcının kimliğini context'e koyar.
func WithUser(ctx context.Context, userID int64) context.Context {
	return context.WithValue(ctx, userKey{}, userID)
}

// UserFrom, context'teki kullanıcı kimliğini döner.
func UserFrom(ctx context.Context) (int64, bool) {
	id, ok := ctx.Value(userKey{}).(int64)
	return id, ok
}

// Begin, bir veritabanı işlemi başlatır ve context'te kullanıcı varsa onu işlemin ömrü boyunca
// "app.user_id" oturum ayarına yazar. Tetikleyiciler denetim kaydında "kim" bilgisini buradan okur.
func Begin(ctx context.Context, pool *pgxpool.Pool) (pgx.Tx, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}

	userID, ok := UserFrom(ctx)
	if !ok {
		return tx, nil
	}

	const setUser = `SELECT set_config('app.user_id', $1, true)`
	if _, err := tx.Exec(ctx, setUser, strconv.FormatInt(userID, 10)); err != nil {
		_ = tx.Rollback(ctx)
		return nil, fmt.Errorf("denetim kullanıcısı ayarlanamadı: %w", err)
	}
	return tx, nil
}
