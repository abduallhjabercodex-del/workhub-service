package middleware

import (
	"context"
	"net/http"
	"strings"

	"workhub-service/internal/infrastructure/auth"
)

type userCtxKey struct{}

type AuthenticatedUser struct {
	ID    string
	Email string
}

func Authenticate(jwtSecret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tokenStr := extractBearerToken(r)
			if tokenStr != "" {
				claims, err := auth.ValidateAccessToken(tokenStr, jwtSecret)
				if err == nil && claims != nil {
					authUser := AuthenticatedUser{
						ID:    claims.Sub,
						Email: claims.Email,
					}
					ctx := context.WithValue(r.Context(), userCtxKey{}, authUser)
					r = r.WithContext(ctx)
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := UserFromContext(r.Context()); !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":{"code":"unauthorized","message":"authentication required"}}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func UserFromContext(ctx context.Context) (AuthenticatedUser, bool) {
	u, ok := ctx.Value(userCtxKey{}).(AuthenticatedUser)
	return u, ok
}

// extractBearerToken extracts token from:
// 1. Authorization: Bearer <token>
// 2. access_token cookie
func extractBearerToken(r *http.Request) string {
	if h := r.Header.Get("Authorization"); h != "" {
		parts := strings.SplitN(h, " ", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			if t := strings.TrimSpace(parts[1]); t != "" {
				return t
			}
		}
	}
	if cookie, err := r.Cookie("access_token"); err == nil && cookie.Value != "" {
		return strings.TrimSpace(cookie.Value)
	}
	return ""
}

