package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"workhub-service/internal/infrastructure/auth"
)

// TestAuthMiddlewareValidAndInvalidToken verifies JWT authentication middleware extracts claims for valid tokens and blocks invalid or unauthenticated requests with 401 Unauthorized.
func TestAuthMiddlewareValidAndInvalidToken(t *testing.T) {
	secret := "middleware-test-secret"
	userID := "505"
	email := "auth@example.com"
	validToken, err := auth.GenerateAccessToken(userID, email, secret, time.Minute)
	if err != nil {
		t.Fatalf("GenerateAccessToken error = %v", err)
	}

	handler := Authenticate(secret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := UserFromContext(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if u.ID != userID || u.Email != email {
			t.Fatalf("user sub=%s email=%s, want %s %s", u.ID, u.Email, userID, email)
		}
		w.WriteHeader(http.StatusOK)
	}))

	// 1. Valid Bearer Token
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+validToken)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, expected %d", rec.Code, http.StatusOK)
	}

	// 2. Invalid Bearer Token
	reqInvalid := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	reqInvalid.Header.Set("Authorization", "Bearer invalid-token-string")
	recInvalid := httptest.NewRecorder()
	handler.ServeHTTP(recInvalid, reqInvalid)
	if recInvalid.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, expected %d", recInvalid.Code, http.StatusUnauthorized)
	}

	// 3. RequireAuth Middleware Blocking Unauthenticated Requests
	protectedHandler := Authenticate(secret)(RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})))

	reqNoAuth := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	recNoAuth := httptest.NewRecorder()
	protectedHandler.ServeHTTP(recNoAuth, reqNoAuth)
	if recNoAuth.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, expected %d", recNoAuth.Code, http.StatusUnauthorized)
	}
}
