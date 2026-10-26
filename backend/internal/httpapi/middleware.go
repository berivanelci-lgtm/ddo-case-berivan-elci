package httpapi

import (
	"context"
	"net/http"
	"strings"

	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/audit"
	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/auth"
	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/user"
)

type claimsContextKey struct{}

// TokenParser, access token doğrulayan her şeyi temsil eder.
type TokenParser interface {
	Parse(tokenString string, kind auth.TokenKind) (*auth.Claims, error)
}

// Authenticate, isteğin geçerli bir access token taşıdığını doğrular.
// Doğrulanan bilgiler isteğin context'ine konur, ClaimsFrom ile okunur.
func Authenticate(parser TokenParser) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := bearerToken(r)
			if !ok {
				writeUnauthorized(w, "missing_token", "Authorization başlığında Bearer token gerekli")
				return
			}

			claims, err := parser.Parse(token, auth.AccessToken)
			if err != nil {
				writeUnauthorized(w, "invalid_token", "token geçersiz veya süresi dolmuş")
				return
			}

			ctx := context.WithValue(r.Context(), claimsContextKey{}, claims)
			ctx = audit.WithUser(ctx, claims.UserID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireRoles, sadece verilen rollerden birine sahip kullanıcıları geçirir.
// Authenticate'den sonra kullanılmalıdır.
func RequireRoles(allowed ...user.Role) func(http.Handler) http.Handler {
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, role := range allowed {
		allowedSet[string(role)] = struct{}{}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := ClaimsFrom(r.Context())
			if !ok {
				writeUnauthorized(w, "missing_token", "kimlik doğrulama gerekli")
				return
			}

			if _, permitted := allowedSet[claims.Role]; !permitted {
				WriteError(w, http.StatusForbidden, "forbidden", "bu işlem için yetkiniz yok")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// ClaimsFrom, Authenticate'in context'e koyduğu token bilgilerini döner.
func ClaimsFrom(ctx context.Context) (*auth.Claims, bool) {
	claims, ok := ctx.Value(claimsContextKey{}).(*auth.Claims)
	return claims, ok
}

// bearerToken, "Authorization: Bearer <token>" başlığından token'ı çıkarır.
func bearerToken(r *http.Request) (string, bool) {
	parts := strings.Fields(r.Header.Get("Authorization"))
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return "", false
	}
	return parts[1], true
}

func writeUnauthorized(w http.ResponseWriter, code, message string) {
	w.Header().Set("WWW-Authenticate", "Bearer")
	WriteError(w, http.StatusUnauthorized, code, message)
}
