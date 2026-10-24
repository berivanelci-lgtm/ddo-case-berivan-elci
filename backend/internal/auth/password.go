// Package auth, parola özeti ve kimlik doğrulama işlemlerini içerir.
package auth

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// HashPassword, parolanın bcrypt özetini üretir.
// Aynı parola her seferinde farklı özet verir, çünkü bcrypt rastgele tuz ekler.
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("parola özeti üretilemedi: %w", err)
	}
	return string(hash), nil
}

// CheckPassword, parolanın verilen özetle eşleşip eşleşmediğini söyler.
func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}
