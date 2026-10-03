package abuse_test

import (
	"bytes"
	"context"
	"encoding/json"
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
	"workhub-service/internal/middleware"
	"workhub-service/internal/repository"
	"workhub-service/internal/service"
)

// TestIdempotencyReplay tests idempotency key caching and replay semantics.
func TestIdempotencyReplay(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	rClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rClient.Close()

	idempotencyStore := cache.NewRedisIdempotencyStore(rClient, 24*time.Hour)

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

	api := handler.NewAPI(svc, nil, idempotencyStore, slog.Default())
	authHandler := middleware.Authenticate(secret)(api.Routes())

	ts := httptest.NewServer(authHandler)
	defer ts.Close()

	ctx := context.Background()

	userAuth, err := svc.Register(ctx, domain.RegisterInput{
		Email:       "idempotent@example.com",
		DisplayName: "Idempotent User",
		Password:    "Password123!",
	})
	if err != nil {
		t.Fatalf("failed to register user: %v", err)
	}

	t.Run("Same Idempotency-Key returns cached response with Idempotent-Replay header", func(t *testing.T) {
		key := "unique-request-key-12345"
		body, _ := json.Marshal(map[string]string{
			"name": "Idempotent Project",
		})

		// First request (New)
		req1, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/projects", bytes.NewReader(body))
		req1.Header.Set("Authorization", "Bearer "+userAuth.AccessToken)
		req1.Header.Set("Content-Type", "application/json")
		req1.Header.Set("Idempotency-Key", key)

		resp1, err := ts.Client().Do(req1)
		if err != nil {
			t.Fatalf("request 1 failed: %v", err)
		}
		defer resp1.Body.Close()

		if resp1.StatusCode != http.StatusCreated {
			t.Fatalf("expected 201 Created on request 1, got %d", resp1.StatusCode)
		}
		if resp1.Header.Get("Idempotent-Replay") == "true" {
			t.Errorf("request 1 should not have Idempotent-Replay header")
		}

		body1, _ := io.ReadAll(resp1.Body)

		// Second request (Replay with same key)
		req2, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/projects", bytes.NewReader(body))
		req2.Header.Set("Authorization", "Bearer "+userAuth.AccessToken)
		req2.Header.Set("Content-Type", "application/json")
		req2.Header.Set("Idempotency-Key", key)

		resp2, err := ts.Client().Do(req2)
		if err != nil {
			t.Fatalf("request 2 failed: %v", err)
		}
		defer resp2.Body.Close()

		if resp2.StatusCode != http.StatusCreated {
			t.Errorf("expected 201 Created on request 2 replay, got %d", resp2.StatusCode)
		}
		if resp2.Header.Get("Idempotent-Replay") != "true" {
			t.Errorf("expected Idempotent-Replay: true header on request 2")
		}

		body2, _ := io.ReadAll(resp2.Body)
		if string(body1) != string(body2) {
			t.Errorf("expected replayed body to match original body.\nGot 1: %s\nGot 2: %s", string(body1), string(body2))
		}
	})

	t.Run("Different Idempotency-Key creates a new project", func(t *testing.T) {
		key1 := "key-diff-1"
		key2 := "key-diff-2"

		body1, _ := json.Marshal(map[string]string{"name": "Project Diff 1"})
		req1, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/projects", bytes.NewReader(body1))
		req1.Header.Set("Authorization", "Bearer "+userAuth.AccessToken)
		req1.Header.Set("Content-Type", "application/json")
		req1.Header.Set("Idempotency-Key", key1)
		resp1, _ := ts.Client().Do(req1)
		resp1.Body.Close()

		body2, _ := json.Marshal(map[string]string{"name": "Project Diff 2"})
		req2, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/projects", bytes.NewReader(body2))
		req2.Header.Set("Authorization", "Bearer "+userAuth.AccessToken)
		req2.Header.Set("Content-Type", "application/json")
		req2.Header.Set("Idempotency-Key", key2)
		resp2, _ := ts.Client().Do(req2)
		resp2.Body.Close()

		if resp2.Header.Get("Idempotent-Replay") == "true" {
			t.Errorf("request with different key should not be a replay")
		}
	})
}
