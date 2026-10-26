package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/auth"
	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/user"
)

// Refresher, refresh token karşılığında yeni token çifti veren her şeydir.
type Refresher interface {
	Refresh(ctx context.Context, refreshToken string) (user.User, auth.TokenPair, error)
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// RefreshHandler, POST /auth/refresh isteğini karşılar. Yanıt, giriş yanıtıyla aynı biçimdedir.
func RefreshHandler(refresher Refresher) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req refreshRequest
		if !decodeBody(w, r, &req) {
			return
		}
		if strings.TrimSpace(req.RefreshToken) == "" {
			WriteError(w, http.StatusBadRequest, "validation_error", "refresh_token zorunludur")
			return
		}

		u, pair, err := refresher.Refresh(r.Context(), req.RefreshToken)
		if errors.Is(err, auth.ErrInvalidToken) {
			writeUnauthorized(w, "invalid_token", "refresh token geçersiz veya süresi dolmuş")
			return
		}
		if err != nil {
			slog.Error("token yenilenemedi", "error", err)
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
