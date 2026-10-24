package user

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound, aranan kullanıcı bulunamadığında döner.
var ErrNotFound = errors.New("kullanıcı bulunamadı")

// Repository, kullanıcıları PostgreSQL'de saklar.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository, yeni bir Repository oluşturur.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// FindByEmail, kullanıcıyı e-posta adresiyle bulur.
func (r *Repository) FindByEmail(ctx context.Context, email string) (User, error) {
	const query = `
		SELECT id, email, password_hash, full_name, role, created_at
		FROM users
		WHERE email = $1`

	var (
		u    User
		role string
	)
	err := r.pool.QueryRow(ctx, query, email).
		Scan(&u.ID, &u.Email, &u.PasswordHash, &u.FullName, &role, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("kullanıcı sorgulanamadı: %w", err)
	}

	u.Role = Role(role)
	return u, nil
}

// CreateIfAbsent, aynı e-postayla kayıt yoksa kullanıcıyı ekler.
// Eklendiyse true, zaten varsa false döner.
func (r *Repository) CreateIfAbsent(ctx context.Context, u User) (bool, error) {
	const query = `
		INSERT INTO users (email, password_hash, full_name, role)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (email) DO NOTHING`

	tag, err := r.pool.Exec(ctx, query, u.Email, u.PasswordHash, u.FullName, string(u.Role))
	if err != nil {
		return false, fmt.Errorf("kullanıcı eklenemedi: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}
