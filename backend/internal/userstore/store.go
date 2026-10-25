// Package userstore, kullanıcıları PostgreSQL'de saklar.
package userstore

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/user"
)

// Store, kullanıcıları PostgreSQL'de saklar.
type Store struct {
	pool *pgxpool.Pool
}

var _ user.Creator = (*Store)(nil)

// New, yeni bir Store oluşturur.
func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// FindByEmail, kullanıcıyı e-posta adresiyle bulur.
func (s *Store) FindByEmail(ctx context.Context, email string) (user.User, error) {
	const query = `
		SELECT id, email, password_hash, full_name, role, created_at
		FROM users
		WHERE email = $1`

	var (
		u    user.User
		role string
	)
	err := s.pool.QueryRow(ctx, query, email).
		Scan(&u.ID, &u.Email, &u.PasswordHash, &u.FullName, &role, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return user.User{}, user.ErrNotFound
	}
	if err != nil {
		return user.User{}, fmt.Errorf("kullanıcı sorgulanamadı: %w", err)
	}

	u.Role = user.Role(role)
	return u, nil
}

// CreateIfAbsent, aynı e-postayla kayıt yoksa kullanıcıyı ekler.
// Eklendiyse true, zaten varsa false döner.
func (s *Store) CreateIfAbsent(ctx context.Context, u user.User) (bool, error) {
	const query = `
		INSERT INTO users (email, password_hash, full_name, role)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (email) DO NOTHING`

	tag, err := s.pool.Exec(ctx, query, u.Email, u.PasswordHash, u.FullName, string(u.Role))
	if err != nil {
		return false, fmt.Errorf("kullanıcı eklenemedi: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}
