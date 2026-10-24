package user

import (
	"context"
	"fmt"
)

// DemoAccount, demo için eklenen örnek bir hesabı tanımlar.
type DemoAccount struct {
	Email    string
	FullName string
	Role     Role
}

// DemoAccounts, her rol için bir demo hesap döner.
func DemoAccounts() []DemoAccount {
	return []DemoAccount{
		{Email: "operator@hatkontrol.test", FullName: "Demo Operatör", Role: RoleOperator},
		{Email: "hatsefi@hatkontrol.test", FullName: "Demo Hat Şefi", Role: RoleHatSefi},
		{Email: "kalite@hatkontrol.test", FullName: "Demo Kalite Mühendisi", Role: RoleKaliteMuhendisi},
		{Email: "planlama@hatkontrol.test", FullName: "Demo Üretim Planlama", Role: RoleUretimPlanlama},
		{Email: "bakim@hatkontrol.test", FullName: "Demo Bakım", Role: RoleBakim},
		{Email: "mudur@hatkontrol.test", FullName: "Demo Fabrika Müdürü", Role: RoleFabrikaMuduru},
	}
}

// Creator, kullanıcıyı yoksa ekleyen her şeyi temsil eder.
type Creator interface {
	CreateIfAbsent(ctx context.Context, u User) (bool, error)
}

// PasswordHasher, parolanın özetini üretir.
type PasswordHasher func(password string) (string, error)

// SeedDemoUsers, eksik demo hesapları ekler ve kaç tanesinin eklendiğini döner.
// Zaten var olan hesaplara dokunmaz, bu yüzden tekrar çalıştırmak güvenlidir.
func SeedDemoUsers(ctx context.Context, creator Creator, hash PasswordHasher, password string) (int, error) {
	passwordHash, err := hash(password)
	if err != nil {
		return 0, fmt.Errorf("demo parola özeti üretilemedi: %w", err)
	}

	created := 0
	for _, account := range DemoAccounts() {
		added, err := creator.CreateIfAbsent(ctx, User{
			Email:        account.Email,
			PasswordHash: passwordHash,
			FullName:     account.FullName,
			Role:         account.Role,
		})
		if err != nil {
			return created, fmt.Errorf("%s eklenemedi: %w", account.Email, err)
		}
		if added {
			created++
		}
	}
	return created, nil
}
