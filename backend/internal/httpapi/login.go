package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/auth"
	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/user"
)

// maxBodyBytes, kabul edilen en büyük istek gövdesidir.
const maxBodyBytes = 1 << 20

// Authenticator, e-posta ve parolayla giriş yapan her şeyi temsil eder.
type Authenticator interface {
	Login(ctx context.Context, email, password string) (user.User, auth.TokenPair, error)
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type userResponse struct {
	ID       int64  `json:"id"`
	Email    string `json:"email"`
	FullName string `json:"full_name"`
	Role     string `json:"role"`
}

type loginResponse struct {
	AccessToken  string       `json:"access_token"`
	RefreshToken string       `json:"refresh_token"`
	TokenType    string       `json:"token_type"`
	User         userResponse `json:"user"`
}

// LoginHandler, POST /auth/login isteğini karşılar.
func LoginHandler(authenticator Authenticator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)

		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()

		var req loginRequest
		if err := decoder.Decode(&req); err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_request", "istek gövdesi geçerli bir JSON değil")
			return
		}

		if strings.TrimSpace(req.Email) == "" || req.Password == "" {
			WriteError(w, http.StatusBadRequest, "validation_error", "e-posta ve parola zorunludur")
			return
		}

		u, pair, err := authenticator.Login(r.Context(), req.Email, req.Password)
		if errors.Is(err, auth.ErrInvalidCredentials) {
			WriteError(w, http.StatusUnauthorized, "invalid_credentials", "e-posta veya parola hatalı")
			return
		}
		if err != nil {
			slog.Error("giriş işlemi başarısız", "error", err)
			WriteError(w, http.StatusInternalServerError, "internal_error", "beklenmeyen bir hata oluştu")
			return
		}

		WriteJSON(w, http.StatusOK, loginResponse{
			AccessToken:  pair.AccessToken,
			RefreshToken: pair.RefreshToken,
			TokenType:    "Bearer",
			User: userResponse{
				ID:       u.ID,
				Email:    u.Email,
				FullName: u.FullName,
				Role:     string(u.Role),
			},
		})
	}
}
