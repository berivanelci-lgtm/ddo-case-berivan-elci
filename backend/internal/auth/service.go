package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/user"
)

// timingPassword, kullanıcı bulunamadığında sahte özet üretmek için kullanılır.
const timingPassword = "zamanlama-icin-sahte-parola"

// ErrInvalidCredentials, e-posta veya parola hatalı olduğunda döner.
// İki durum için de aynı hata kullanılır, böylece kayıtlı e-postalar sızmaz.
var ErrInvalidCredentials = errors.New("e-posta veya parola hatalı")

// UserFinder, kullanıcıyı e-postayla ya da kimlikle bulan her şeyi temsil eder.
type UserFinder interface {
	FindByEmail(ctx context.Context, email string) (user.User, error)
	FindByID(ctx context.Context, id int64) (user.User, error)
}

// TokenPair, giriş sonrasında verilen iki tokenı tutar.
type TokenPair struct {
	AccessToken  string
	RefreshToken string
}

// Service, giriş ve token yenileme işlemlerini yürütür.
type Service struct {
	users     UserFinder
	tokens    *TokenManager
	dummyHash string
}

// NewService, yeni bir giriş servisi oluşturur.
func NewService(users UserFinder, tokens *TokenManager) (*Service, error) {
	dummyHash, err := HashPassword(timingPassword)
	if err != nil {
		return nil, fmt.Errorf("sahte özet üretilemedi: %w", err)
	}
	return &Service{users: users, tokens: tokens, dummyHash: dummyHash}, nil
}

// Login, bilgiler doğruysa kullanıcıyı ve token çiftini döner.
func (s *Service) Login(ctx context.Context, email, password string) (user.User, TokenPair, error) {
	normalized := strings.ToLower(strings.TrimSpace(email))

	u, err := s.users.FindByEmail(ctx, normalized)
	if errors.Is(err, user.ErrNotFound) {
		// Kullanıcı yokken de parola kontrolü yapılır, cevap süresi birbirine yakın kalır.
		CheckPassword(s.dummyHash, password)
		return user.User{}, TokenPair{}, ErrInvalidCredentials
	}
	if err != nil {
		return user.User{}, TokenPair{}, fmt.Errorf("kullanıcı aranamadı: %w", err)
	}

	if !CheckPassword(u.PasswordHash, password) {
		return user.User{}, TokenPair{}, ErrInvalidCredentials
	}

	pair, err := s.issuePair(u)
	if err != nil {
		return user.User{}, TokenPair{}, err
	}
	return u, pair, nil
}

// Refresh, geçerli bir refresh token karşılığında yeni bir token çifti verir.
// Rol token'dan değil veritabanından okunur: rolü değişen kullanıcı yeni rolüyle,
// silinen kullanıcı hiç token alamaz.
func (s *Service) Refresh(ctx context.Context, refreshToken string) (user.User, TokenPair, error) {
	claims, err := s.tokens.Parse(refreshToken, RefreshToken)
	if err != nil {
		return user.User{}, TokenPair{}, ErrInvalidToken
	}

	u, err := s.users.FindByID(ctx, claims.UserID)
	if errors.Is(err, user.ErrNotFound) {
		return user.User{}, TokenPair{}, ErrInvalidToken
	}
	if err != nil {
		return user.User{}, TokenPair{}, fmt.Errorf("kullanıcı aranamadı: %w", err)
	}

	pair, err := s.issuePair(u)
	if err != nil {
		return user.User{}, TokenPair{}, err
	}
	return u, pair, nil
}

func (s *Service) issuePair(u user.User) (TokenPair, error) {
	access, err := s.tokens.Issue(u.ID, string(u.Role), AccessToken)
	if err != nil {
		return TokenPair{}, err
	}
	refresh, err := s.tokens.Issue(u.ID, string(u.Role), RefreshToken)
	if err != nil {
		return TokenPair{}, err
	}
	return TokenPair{AccessToken: access, RefreshToken: refresh}, nil
}
