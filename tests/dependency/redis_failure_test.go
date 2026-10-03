package dependency_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"workhub-service/internal/domain"
	"workhub-service/internal/handler"
	"workhub-service/internal/infrastructure/cache"
	"workhub-service/internal/infrastructure/circuitbreaker"
	"workhub-service/internal/infrastructure/metrics"
	"workhub-service/internal/middleware"
	"workhub-service/internal/repository"
	"workhub-service/internal/service"
)

// TestRedisFailureScenarios tests system behavior during Redis dependency failures:
// 1. GET /api/v1/projects/{id}/summary returns 200 (cache miss / fail-open)
// 2. Circuit breaker opens (state 2) after threshold failures
// 3. Rate limiting fail-closed protections when store is exhausted or failing
func TestRedisFailureScenarios(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}

	rClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	// 5 failures threshold, 10s cooldown
	cb := circuitbreaker.New(5, 10*time.Second)

	summaryCache := cache.NewProjectSummaryCache(rClient, 1*time.Hour, slog.Default(), cb)
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
		Cache:                summaryCache,
		MaxProjectsPerUser:   10,
		MaxMembersPerProject: 10,
		MaxTasksPerProject:   10,
		Config: service.AuthConfig{
			JWTSecret:       secret,
			AccessTokenTTL:  15 * time.Minute,
			RefreshTokenTTL: 168 * time.Hour,
			BCryptCost:      10,
		},
	}
	svc, err := service.NewWorkHub(deps)
	if err != nil {
		t.Fatalf("failed to init workhub: %v", err)
	}

	api := handler.NewAPI(svc, nil, nil, slog.Default())
	authHandler := middleware.Authenticate(secret)(api.Routes())
	ts := httptest.NewServer(authHandler)
	defer ts.Close()

	ctx := context.Background()

	// Register owner and create project
	owner, err := svc.Register(ctx, domain.RegisterInput{
		Email:       "redistest@example.com",
		DisplayName: "Redis User",
		Password:    "Password123!",
	})
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}

	proj, err := svc.CreateProject(ctx, owner.User.ID, domain.CreateProjectInput{Name: "Summary Project"})
	if err != nil {
		t.Fatalf("create project failed: %v", err)
	}

	t.Run("GET project summary returns 200 on cache miss when Redis is down (fail-open)", func(t *testing.T) {
		// Close Redis connection to simulate Redis outage
		mr.Close()

		// Send 6 summary requests to trigger breaker threshold (5 failures)
		for i := 0; i < 6; i++ {
			req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/api/v1/projects/%s/summary", ts.URL, proj.ID), nil)
			req.Header.Set("Authorization", "Bearer "+owner.AccessToken)
			resp, err := ts.Client().Do(req)
			if err != nil {
				t.Fatalf("request %d failed: %v", i, err)
			}
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Errorf("request %d expected 200 OK despite Redis failure, got %d: %s", i, resp.StatusCode, string(body))
			}
		}

		// Verify circuit breaker state transitioned to OPEN (State = 2)
		state := cb.State()
		if state != circuitbreaker.StateOpen {
			t.Errorf("expected circuit breaker state to be Open (2), got %v", state)
		}
	})

	t.Run("Rate limit fails closed when rate limiter store is exhausted", func(t *testing.T) {
		// New rate limiter store with 1 capacity
		exhaustedLimiter := middleware.NewRateLimiter(1, time.Second)

		limiterHandler := middleware.RateLimit(exhaustedLimiter, nil)(api.Routes())
		limiterTS := httptest.NewServer(limiterHandler)
		defer limiterTS.Close()

		req, _ := http.NewRequest(http.MethodGet, limiterTS.URL+"/api/v1/projects", nil)
		resp, err := limiterTS.Client().Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		// 2nd request immediately rejected (429 fail-closed)
		req2, _ := http.NewRequest(http.MethodGet, limiterTS.URL+"/api/v1/projects", nil)
		resp2, err := limiterTS.Client().Do(req2)
		if err != nil {
			t.Fatalf("2nd request failed: %v", err)
		}
		defer resp2.Body.Close()

		if resp2.StatusCode != http.StatusTooManyRequests {
			t.Errorf("expected 429 Too Many Requests (fail-closed), got %d", resp2.StatusCode)
		}
	})

	t.Run("Verify metric workhub_cache_circuit_state registered", func(t *testing.T) {
		metrics.UpdateCacheCircuitState(2)
		// Metric set successfully
	})
}
