package user

import (
	"context"
	"errors"
	"testing"
)

// fakeCreator, veritabanı yerine bellekte kullanıcı tutan sahte bir Creator'dır.
type fakeCreator struct {
	users map[string]User
	err   error
}

func newFakeCreator() *fakeCreator {
	return &fakeCreator{users: make(map[string]User)}
}

func (f *fakeCreator) CreateIfAbsent(_ context.Context, u User) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	if _, exists := f.users[u.Email]; exists {
		return false, nil
	}
	f.users[u.Email] = u
	return true, nil
}

func fakeHasher(password string) (string, error) {
	return "ozet:" + password, nil
}

func TestDemoAccounts_CoverEveryRoleOnce(t *testing.T) {
	seen := make(map[Role]int)
	for _, account := range DemoAccounts() {
		seen[account.Role]++
	}
	for _, role := range AllRoles() {
		if seen[role] != 1 {
			t.Errorf("%q rolü için tam 1 demo hesap beklenirdi, gelen %d", role, seen[role])
		}
	}
}

func TestDemoAccounts_HaveUniqueEmails(t *testing.T) {
	seen := make(map[string]bool)
	for _, account := range DemoAccounts() {
		if seen[account.Email] {
			t.Errorf("e-posta tekrar ediyor: %s", account.Email)
		}
		seen[account.Email] = true
	}
}

func TestSeedDemoUsers_CreatesAllAccounts(t *testing.T) {
	creator := newFakeCreator()

	created, err := SeedDemoUsers(context.Background(), creator, fakeHasher, "parola")
	if err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}
	if created != 6 {
		t.Fatalf("6 hesap eklenmeli, eklenen %d", created)
	}
}

func TestSeedDemoUsers_IsIdempotent(t *testing.T) {
	creator := newFakeCreator()

	if _, err := SeedDemoUsers(context.Background(), creator, fakeHasher, "parola"); err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}
	created, err := SeedDemoUsers(context.Background(), creator, fakeHasher, "parola")
	if err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}
	if created != 0 {
		t.Fatalf("ikinci çalıştırmada yeni hesap eklenmemeli, eklenen %d", created)
	}
	if len(creator.users) != 6 {
		t.Fatalf("toplam 6 kullanıcı kalmalı, gelen %d", len(creator.users))
	}
}

func TestSeedDemoUsers_StoresHashNotPlainPassword(t *testing.T) {
	creator := newFakeCreator()

	if _, err := SeedDemoUsers(context.Background(), creator, fakeHasher, "parola"); err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}
	for email, u := range creator.users {
		if u.PasswordHash == "parola" {
			t.Errorf("%s için düz parola saklanmış", email)
		}
	}
}

func TestSeedDemoUsers_PropagatesCreatorError(t *testing.T) {
	creator := newFakeCreator()
	creator.err = errors.New("veritabanı hatası")

	if _, err := SeedDemoUsers(context.Background(), creator, fakeHasher, "parola"); err == nil {
		t.Fatal("ekleme hatası yukarı iletilmeli")
	}
}

func TestSeedDemoUsers_PropagatesHashError(t *testing.T) {
	failingHasher := func(string) (string, error) { return "", errors.New("özet hatası") }

	if _, err := SeedDemoUsers(context.Background(), newFakeCreator(), failingHasher, "parola"); err == nil {
		t.Fatal("özet hatası yukarı iletilmeli")
	}
}
