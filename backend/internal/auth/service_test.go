package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/user"
)

// fakeFinder, veritabanı yerine bellekte kullanıcı tutan sahte bir UserFinder'dır.
type fakeFinder struct {
	users map[string]user.User
	err   error
}

func (f *fakeFinder) FindByEmail(_ context.Context, email string) (user.User, error) {
	if f.err != nil {
		return user.User{}, f.err
	}
	u, ok := f.users[email]
	if !ok {
		return user.User{}, user.ErrNotFound
	}
	return u, nil
}

func newTestService(t *testing.T, finder UserFinder) *Service {
	t.Helper()
	service, err := NewService(finder, newTestManager(t))
	if err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}
	return service
}

func newFinderWithUser(t *testing.T) *fakeFinder {
	t.Helper()
	hash, err := HashPassword("dogru-parola")
	if err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}
	return &fakeFinder{users: map[string]user.User{
		"hatsefi@hatkontrol.test": {
			ID:           7,
			Email:        "hatsefi@hatkontrol.test",
			PasswordHash: hash,
			FullName:     "Demo Hat Şefi",
			Role:         user.RoleHatSefi,
		},
	}}
}

func TestLogin_Success(t *testing.T) {
	service := newTestService(t, newFinderWithUser(t))

	u, pair, err := service.Login(context.Background(), "hatsefi@hatkontrol.test", "dogru-parola")
	if err != nil {
		t.Fatalf("doğru bilgilerle giriş başarılı olmalı: %v", err)
	}
	if u.ID != 7 {
		t.Errorf("kullanıcı 7 olmalı, gelen %d", u.ID)
	}

	claims, err := service.tokens.Parse(pair.AccessToken, AccessToken)
	if err != nil {
		t.Fatalf("access token geçerli olmalı: %v", err)
	}
	if claims.Role != string(user.RoleHatSefi) {
		t.Errorf("token rolü hat_sefi olmalı, gelen %q", claims.Role)
	}
	if _, err := service.tokens.Parse(pair.RefreshToken, RefreshToken); err != nil {
		t.Errorf("refresh token geçerli olmalı: %v", err)
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	service := newTestService(t, newFinderWithUser(t))

	_, _, err := service.Login(context.Background(), "hatsefi@hatkontrol.test", "yanlis-parola")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("ErrInvalidCredentials beklenirdi, gelen: %v", err)
	}
}

func TestLogin_UnknownEmail_GivesSameErrorAsWrongPassword(t *testing.T) {
	service := newTestService(t, newFinderWithUser(t))

	_, _, err := service.Login(context.Background(), "yok@hatkontrol.test", "dogru-parola")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("kayıtsız e-posta da ErrInvalidCredentials vermeli, gelen: %v", err)
	}
}

func TestLogin_NormalizesEmail(t *testing.T) {
	service := newTestService(t, newFinderWithUser(t))

	if _, _, err := service.Login(context.Background(), "  HatSefi@HatKontrol.test ", "dogru-parola"); err != nil {
		t.Fatalf("büyük harf ve boşluk fark etmemeli: %v", err)
	}
}

func TestLogin_RepositoryErrorIsNotReportedAsBadCredentials(t *testing.T) {
	service := newTestService(t, &fakeFinder{err: errors.New("veritabanı kapalı")})

	_, _, err := service.Login(context.Background(), "hatsefi@hatkontrol.test", "dogru-parola")
	if err == nil {
		t.Fatal("veritabanı hatası yukarı iletilmeli")
	}
	if errors.Is(err, ErrInvalidCredentials) {
		t.Fatal("sunucu hatası, hatalı kimlik bilgisi gibi gösterilmemeli")
	}
}
