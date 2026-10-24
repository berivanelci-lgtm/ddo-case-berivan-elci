package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const minSecretLength = 32

// TokenKind, tokenın access mi refresh mi olduğunu belirtir.
type TokenKind string

const (
	AccessToken  TokenKind = "access"
	RefreshToken TokenKind = "refresh"
)

// ErrInvalidToken, token bozuk, süresi dolmuş veya yanlış türdeyse döner.
var ErrInvalidToken = errors.New("geçersiz veya süresi dolmuş token")

// Claims, tokenın içinde taşınan bilgilerdir.
type Claims struct {
	UserID int64     `json:"uid"`
	Role   string    `json:"role"`
	Kind   TokenKind `json:"kind"`
	jwt.RegisteredClaims
}

// TokenManager, token üretir ve doğrular.
type TokenManager struct {
	secret     []byte
	accessTTL  time.Duration
	refreshTTL time.Duration
	now        func() time.Time
}

// NewTokenManager, yeni bir TokenManager oluşturur.
// Gizli anahtar en az 32 karakter olmalıdır.
func NewTokenManager(secret string, accessTTL, refreshTTL time.Duration) (*TokenManager, error) {
	if len(secret) < minSecretLength {
		return nil, fmt.Errorf("JWT gizli anahtarı en az %d karakter olmalı", minSecretLength)
	}
	return &TokenManager{
		secret:     []byte(secret),
		accessTTL:  accessTTL,
		refreshTTL: refreshTTL,
		now:        time.Now,
	}, nil
}

// Issue, verilen kullanıcı için imzalı bir token üretir.
func (m *TokenManager) Issue(userID int64, role string, kind TokenKind) (string, error) {
	var ttl time.Duration
	switch kind {
	case AccessToken:
		ttl = m.accessTTL
	case RefreshToken:
		ttl = m.refreshTTL
	default:
		return "", fmt.Errorf("bilinmeyen token türü: %q", kind)
	}

	now := m.now()
	claims := Claims{
		UserID: userID,
		Role:   role,
		Kind:   kind,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}

	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.secret)
	if err != nil {
		return "", fmt.Errorf("token imzalanamadı: %w", err)
	}
	return signed, nil
}

// Parse, tokenı doğrular ve içindeki bilgileri döner.
// Beklenen türle token türü uyuşmazsa ErrInvalidToken döner.
func (m *TokenManager) Parse(tokenString string, kind TokenKind) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(
		tokenString,
		claims,
		func(*jwt.Token) (any, error) { return m.secret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithTimeFunc(m.now),
		jwt.WithExpirationRequired(),
	)
	if err != nil || !token.Valid {
		return nil, ErrInvalidToken
	}
	if claims.Kind != kind {
		return nil, ErrInvalidToken
	}
	return claims, nil
}
