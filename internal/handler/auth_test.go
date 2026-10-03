package handler

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"workhub-service/internal/domain"
	"workhub-service/internal/middleware"
	"workhub-service/internal/repository"
	"workhub-service/internal/service"
)

func newTestAuthAPI() (*API, *service.WorkHub) {
	repo := repository.NewMemoryRepository()
	deps := service.Deps{
		Projects:    repo,
		Tasks:       repo,
		Summaries:   repo,
		Users:       repo,
		Auth:        repo,
		Memberships: repo,
		Quotas:      repo,
		Pinger:      repo,
		Config: service.AuthConfig{
			JWTSecret:       "test-jwt-secret-key-at-least-32-chars-long!",
			AccessTokenTTL:  15 * time.Minute,
			RefreshTokenTTL: 168 * time.Hour,
			BCryptCost:      10,
		},
	}
	s, err := service.NewWorkHub(deps)
	if err != nil {
		panic(err)
	}
	return NewAPI(s, nil, nil, slog.Default()), s
}

// TestAuthRegistrationLoginRefreshLogoutFlow tests the complete authentication lifecycle across HTTP endpoints: registration, duplicate email prevention, login, user profile query (/me), token rotation, replay prevention of consumed tokens, and logout.
func TestAuthRegistrationLoginRefreshLogoutFlow(t *testing.T) {
	api, _ := newTestAuthAPI()
	handlerStack := middleware.Authenticate("test-jwt-secret-key-at-least-32-chars-long!")(api.Routes())

	// 1. Register
	regBody := `{"email":"testuser@example.com","displayName":"Test User","password":"SecurePassword123"}`
	reqReg := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewBufferString(regBody))
	reqReg.Header.Set("Content-Type", "application/json")
	recReg := httptest.NewRecorder()
	handlerStack.ServeHTTP(recReg, reqReg)

	if recReg.Code != http.StatusCreated {
		t.Fatalf("register status = %d, want %d", recReg.Code, http.StatusCreated)
	}

	var regRes struct {
		Data domain.AuthResponse `json:"data"`
	}
	if err := json.Unmarshal(recReg.Body.Bytes(), &regRes); err != nil {
		t.Fatalf("unmarshal register response: %v", err)
	}
	if regRes.Data.AccessToken == "" || regRes.Data.RefreshToken == "" {
		t.Fatal("tokens in register response are empty")
	}

	// 2. Duplicate Registration -> 409 Conflict
	reqRegDup := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewBufferString(regBody))
	reqRegDup.Header.Set("Content-Type", "application/json")
	recRegDup := httptest.NewRecorder()
	handlerStack.ServeHTTP(recRegDup, reqRegDup)

	if recRegDup.Code != http.StatusConflict {
		t.Fatalf("duplicate register status = %d, want %d", recRegDup.Code, http.StatusConflict)
	}

	// 3. Login
	loginBody := `{"email":"testuser@example.com","password":"SecurePassword123"}`
	reqLogin := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(loginBody))
	reqLogin.Header.Set("Content-Type", "application/json")
	recLogin := httptest.NewRecorder()
	handlerStack.ServeHTTP(recLogin, reqLogin)

	if recLogin.Code != http.StatusOK {
		t.Fatalf("login status = %d, want %d", recLogin.Code, http.StatusOK)
	}

	var loginRes struct {
		Data domain.AuthResponse `json:"data"`
	}
	if err := json.Unmarshal(recLogin.Body.Bytes(), &loginRes); err != nil {
		t.Fatalf("unmarshal login response: %v", err)
	}

	// 4. Get Authenticated User (/api/v1/auth/me)
	reqMe := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	reqMe.RemoteAddr = "192.168.1.2:1234"
	reqMe.Header.Set("Authorization", "Bearer "+loginRes.Data.AccessToken)
	recMe := httptest.NewRecorder()
	handlerStack.ServeHTTP(recMe, reqMe)

	if recMe.Code != http.StatusOK {
		t.Fatalf("me status = %d, want %d", recMe.Code, http.StatusOK)
	}

	// 5. Refresh Token (Rotation)
	refreshBody, _ := json.Marshal(domain.RefreshInput{RefreshToken: loginRes.Data.RefreshToken})
	reqRefresh := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", bytes.NewBuffer(refreshBody))
	reqRefresh.RemoteAddr = "192.168.1.3:1234"
	reqRefresh.Header.Set("Content-Type", "application/json")
	recRefresh := httptest.NewRecorder()
	handlerStack.ServeHTTP(recRefresh, reqRefresh)

	if recRefresh.Code != http.StatusOK {
		t.Fatalf("refresh status = %d, want %d", recRefresh.Code, http.StatusOK)
	}

	var refreshRes struct {
		Data domain.AuthResponse `json:"data"`
	}
	if err := json.Unmarshal(recRefresh.Body.Bytes(), &refreshRes); err != nil {
		t.Fatalf("unmarshal refresh response: %v", err)
	}

	// 6. Reuse Revoked Old Refresh Token -> Expect 401 Unauthorized
	reqReuse := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", bytes.NewBuffer(refreshBody))
	reqReuse.RemoteAddr = "192.168.1.4:1234"
	reqReuse.Header.Set("Content-Type", "application/json")
	recReuse := httptest.NewRecorder()
	handlerStack.ServeHTTP(recReuse, reqReuse)

	if recReuse.Code != http.StatusUnauthorized {
		t.Fatalf("reusing revoked refresh token status = %d, want %d", recReuse.Code, http.StatusUnauthorized)
	}

	// 7. Logout with New Refresh Token
	logoutBody, _ := json.Marshal(domain.LogoutInput{RefreshToken: refreshRes.Data.RefreshToken})
	reqLogout := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", bytes.NewBuffer(logoutBody))
	reqLogout.RemoteAddr = "192.168.1.5:1234"
	reqLogout.Header.Set("Content-Type", "application/json")
	recLogout := httptest.NewRecorder()
	handlerStack.ServeHTTP(recLogout, reqLogout)

	if recLogout.Code != http.StatusNoContent {
		t.Fatalf("logout status = %d, want %d", recLogout.Code, http.StatusNoContent)
	}
}
