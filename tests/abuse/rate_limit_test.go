package abuse_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"workhub-service/internal/domain"
	"workhub-service/internal/handler"
	"workhub-service/internal/middleware"
	"workhub-service/internal/repository"
	"workhub-service/internal/service"
)

// TestRateLimiting tests all Rate Limiting (429) scenarios specified in Day 15 requirements.
func TestRateLimiting(t *testing.T) {
	repo := repository.NewMemoryRepository()
	secret := "test-jwt-secret-key-at-least-32-chars-long!"
	deps := service.Deps{
		Projects:             repo,
		Tasks:                repo,
		Summaries:            repo,
		Users:                repo,
		Auth:                 repo,
		Memberships:          repo,
		Quotas:               repo,
		Pinger:               repo,
		MaxProjectsPerUser:   20,
		MaxMembersPerProject: 20,
		MaxTasksPerProject:   20,
		Config: service.AuthConfig{
			JWTSecret:       secret,
			AccessTokenTTL:  15 * time.Minute,
			RefreshTokenTTL: 168 * time.Hour,
			BCryptCost:      10,
		},
	}
	svc, err := service.NewWorkHub(deps)
	if err != nil {
		t.Fatalf("service init failed: %v", err)
	}

	api := handler.NewAPI(svc, nil, nil, slog.Default())

	// Set up strict rate limiters for testing specific thresholds:
	// Capacity 5 for login tests (6th fails)
	loginLimiter := middleware.NewRateLimiter(5, 10*time.Minute)
	// Capacity 10 for projects creation tests (11th fails)
	projectLimiter := middleware.NewRateLimiter(10, 10*time.Minute)

	router := api.Routes()
	authHandler := middleware.Authenticate(secret)(router)
	rateLimitedHandler := middleware.RateLimit(projectLimiter, nil)(authHandler)

	ts := httptest.NewServer(rateLimitedHandler)
	defer ts.Close()

	userAuth, err := svc.Register(nil, domain.RegisterInput{
		Email:       "ratelimit@example.com",
		DisplayName: "Rate User",
		Password:    "Password123!",
	})
	if err != nil {
		t.Fatalf("failed to register user: %v", err)
	}

	t.Run("Login rate limiting: 6th request gets 429 and Retry-After header", func(t *testing.T) {
		loginHandler := middleware.RateLimit(loginLimiter, nil)(router)
		loginTS := httptest.NewServer(loginHandler)
		defer loginTS.Close()

		loginBody, _ := json.Marshal(map[string]string{
			"email":    "ratelimit@example.com",
			"password": "Password123!",
		})

		for i := 1; i <= 5; i++ {
			req, _ := http.NewRequest(http.MethodPost, loginTS.URL+"/api/v1/auth/login", bytes.NewReader(loginBody))
			req.Header.Set("Content-Type", "application/json")
			resp, err := loginTS.Client().Do(req)
			if err != nil {
				t.Fatalf("request %d failed: %v", i, err)
			}
			resp.Body.Close()

			if resp.Header.Get("X-RateLimit-Limit") == "" {
				t.Errorf("request %d missing X-RateLimit-Limit header", i)
			}
			if resp.StatusCode != http.StatusOK {
				t.Errorf("request %d expected 200, got %d", i, resp.StatusCode)
			}
		}

		// 6th request must fail with 429
		req, _ := http.NewRequest(http.MethodPost, loginTS.URL+"/api/v1/auth/login", bytes.NewReader(loginBody))
		req.Header.Set("Content-Type", "application/json")
		resp, err := loginTS.Client().Do(req)
		if err != nil {
			t.Fatalf("6th request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusTooManyRequests {
			t.Errorf("expected 429 Too Many Requests on 6th attempt, got %d", resp.StatusCode)
		}
		if resp.Header.Get("Retry-After") == "" {
			t.Errorf("missing Retry-After header on 429 response")
		}
		if resp.Header.Get("X-RateLimit-Remaining") != "0" {
			t.Errorf("expected X-RateLimit-Remaining to be 0, got %s", resp.Header.Get("X-RateLimit-Remaining"))
		}
	})

	t.Run("Projects creation rate limiting: 11th request gets 429 and headers present", func(t *testing.T) {
		for i := 1; i <= 10; i++ {
			body, _ := json.Marshal(map[string]string{
				"name": fmt.Sprintf("Project %d", i),
			})
			req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/projects", bytes.NewReader(body))
			req.Header.Set("Authorization", "Bearer "+userAuth.AccessToken)
			req.Header.Set("Content-Type", "application/json")
			resp, err := ts.Client().Do(req)
			if err != nil {
				t.Fatalf("request %d failed: %v", i, err)
			}
			resp.Body.Close()

			if resp.Header.Get("X-RateLimit-Limit") == "" || resp.Header.Get("X-RateLimit-Remaining") == "" {
				t.Errorf("request %d missing X-RateLimit headers", i)
			}
			if resp.StatusCode != http.StatusCreated {
				t.Errorf("request %d expected 201 Created, got %d", i, resp.StatusCode)
			}
		}

		// 11th request must fail with 429
		body, _ := json.Marshal(map[string]string{
			"name": "Project 11",
		})
		req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/projects", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+userAuth.AccessToken)
		req.Header.Set("Content-Type", "application/json")
		resp, err := ts.Client().Do(req)
		if err != nil {
			t.Fatalf("11th request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusTooManyRequests {
			t.Errorf("expected 429 Too Many Requests on 11th creation, got %d", resp.StatusCode)
		}
		if resp.Header.Get("X-RateLimit-Limit") != "10" {
			t.Errorf("expected X-RateLimit-Limit header to be 10, got %s", resp.Header.Get("X-RateLimit-Limit"))
		}
	})
}
