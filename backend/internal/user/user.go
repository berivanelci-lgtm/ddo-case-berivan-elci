package user

import "time"

// User, veritabanındaki bir kullanıcı kaydını temsil eder.
type User struct {
	ID           int64
	Email        string
	PasswordHash string
	FullName     string
	Role         Role
	CreatedAt    time.Time
}
