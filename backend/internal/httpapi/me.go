package httpapi

import "net/http"

type meResponse struct {
	ID   int64  `json:"id"`
	Role string `json:"role"`
}

// MeHandler, token'daki kullanıcı bilgisini döner. Authenticate arkasında kullanılır.
func MeHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := ClaimsFrom(r.Context())
		if !ok {
			writeUnauthorized(w, "missing_token", "kimlik doğrulama gerekli")
			return
		}
		WriteJSON(w, http.StatusOK, meResponse{ID: claims.UserID, Role: claims.Role})
	}
}
