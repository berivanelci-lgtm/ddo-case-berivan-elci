package user

import (
	"errors"
	"time"
)

// ErrNotFound, aranan kullanıcı bulunamadığında döner.
var ErrNotFound = errors.New("kullanıcı bulunamadı")

// User, veritabanındaki bir kullanıcı kaydını temsil eder.
type User struct {
	ID           int64
	Email        string
	PasswordHash string
	FullName     string
	Role         Role
	CreatedAt    time.Time
}
