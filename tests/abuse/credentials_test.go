package abuse_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"workhub-service/internal/domain"
	"workhub-service/internal/infrastructure/auth"
)

// TestInvalidCredentials tests all 401 Unauthorized / Token scenarios listed in Day 15 requirements.
func TestInvalidCredentials(t *testing.T) {
	handlerStack, svc, secret := setupAbuseApp(t)
	ts := httptest.NewServer(handlerStack)
	defer ts.Close()

	ctx := context.Background()

	// Register test user
	userAuth, err := svc.Register(ctx, domain.RegisterInput{
		Email:       "credtest@example.com",
		DisplayName: "Cred Test",
		Password:    "Password123!",
	})
	if err != nil {
		t.Fatalf("failed to register user: %v", err)
	}

	t.Run("Missing Authorization Header -> 401", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/projects", nil)
		resp, err := ts.Client().Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected 401 Unauthorized, got %d", resp.StatusCode)
		}
	})

	t.Run("Expired JWT -> 401 token_expired", func(t *testing.T) {
		expiredToken, err := auth.GenerateAccessToken(userAuth.User.ID, userAuth.User.Email, secret, -10*time.Minute)
		if err != nil {
			t.Fatalf("failed to generate expired token: %v", err)
		}

		req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/projects", nil)
		req.Header.Set("Authorization", "Bearer "+expiredToken)
		resp, err := ts.Client().Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected 401 Unauthorized, got %d", resp.StatusCode)
		}
	})

	t.Run("Tampered JWT -> 401 unauthorized", func(t *testing.T) {
		validToken, err := auth.GenerateAccessToken(userAuth.User.ID, userAuth.User.Email, secret, 15*time.Minute)
		if err != nil {
			t.Fatalf("failed to generate token: %v", err)
		}
		tamperedToken := validToken + "tampered"

		req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/projects", nil)
		req.Header.Set("Authorization", "Bearer "+tamperedToken)
		resp, err := ts.Client().Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected 401 Unauthorized, got %d", resp.StatusCode)
		}
	})

	t.Run("alg: none JWT -> 401 unauthorized", func(t *testing.T) {
		header := map[string]string{"alg": "none", "typ": "JWT"}
		claims := map[string]interface{}{
			"sub":   userAuth.User.ID,
			"email": userAuth.User.Email,
			"iat":   time.Now().Unix(),
			"exp":   time.Now().Add(15 * time.Minute).Unix(),
		}
		hBytes, _ := json.Marshal(header)
		cBytes, _ := json.Marshal(claims)
		noneToken := base64.RawURLEncoding.EncodeToString(hBytes) + "." + base64.RawURLEncoding.EncodeToString(cBytes) + "."

		req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/projects", nil)
		req.Header.Set("Authorization", "Bearer "+noneToken)
		resp, err := ts.Client().Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected 401 Unauthorized, got %d", resp.StatusCode)
		}
	})

	t.Run("iat in the future -> 401 unauthorized", func(t *testing.T) {
		claims := auth.JWTClaims{
			Sub:   userAuth.User.ID,
			Email: userAuth.User.Email,
			Iat:   time.Now().Add(10 * time.Hour).Unix(),
			Exp:   time.Now().Add(11 * time.Hour).Unix(),
			Iss:   auth.TokenIssuer,
			Aud:   auth.TokenAudience,
		}
		hBytes, _ := json.Marshal(map[string]string{"alg": "HS256", "typ": "JWT"})
		cBytes, _ := json.Marshal(claims)
		unsigned := base64.RawURLEncoding.EncodeToString(hBytes) + "." + base64.RawURLEncoding.EncodeToString(cBytes)
		futureToken := unsigned + ".fake_sig"

		req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/projects", nil)
		req.Header.Set("Authorization", "Bearer "+futureToken)
		resp, err := ts.Client().Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected 401 Unauthorized, got %d", resp.StatusCode)
		}
	})

	t.Run("Refresh with Revoked/Reused Token Replay -> 401 + Sessions Revoked", func(t *testing.T) {
		// 1. Initial login to get initial refresh token
		loginRes, err := svc.Login(ctx, domain.LoginInput{
			Email:    "credtest@example.com",
			Password: "Password123!",
		})
		if err != nil {
			t.Fatalf("login failed: %v", err)
		}

		// 2. Perform legitimate refresh (rotates token)
		refreshedRes, err := svc.RefreshToken(ctx, loginRes.RefreshToken)
		if err != nil {
			t.Fatalf("first refresh failed: %v", err)
		}

		// 3. Attempt replay with old refresh token -> Should fail 401 & trigger replay detection
		body, _ := json.Marshal(map[string]string{
			"refreshToken": loginRes.RefreshToken,
		})
		req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/auth/refresh", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := ts.Client().Do(req)
		if err != nil {
			t.Fatalf("refresh replay request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected 401 Unauthorized on token replay, got %d", resp.StatusCode)
		}

		// 4. Verify that the new refreshed token is now also revoked due to family revocation on replay!
		_, err = svc.RefreshToken(ctx, refreshedRes.RefreshToken)
		if err == nil {
			t.Errorf("expected error after replay detection family revocation, but token refresh succeeded")
		}
	})
}
