package abuse_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"workhub-service/internal/domain"
	"workhub-service/internal/handler"
	"workhub-service/internal/middleware"
	"workhub-service/internal/repository"
	"workhub-service/internal/service"
)

// TestQuotaEnforcement tests project quota limit enforcement and concurrent project creation race safety.
func TestQuotaEnforcement(t *testing.T) {
	repo := repository.NewMemoryRepository()
	secret := "test-jwt-secret-key-at-least-32-chars-long!"
	deps := service.Deps{
		Projects:             repo,
		Tasks:                repo,
		Users:                repo,
		Auth:                 repo,
		Memberships:          repo,
		Quotas:               repo,
		MaxProjectsPerUser:   5, // Set quota limit to 5
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

	userAuth, err := svc.Register(ctx, domain.RegisterInput{
		Email:       "quotatest@example.com",
		DisplayName: "Quota User",
		Password:    "Password123!",
	})
	if err != nil {
		t.Fatalf("failed to register user: %v", err)
	}

	t.Run("Create projects up to limit 5 -> 6th returns 403 quota_exceeded", func(t *testing.T) {
		// Fill quota up to limit 5
		for i := 1; i <= 5; i++ {
			body, _ := json.Marshal(map[string]string{
				"name": fmt.Sprintf("Sequential Project %d", i),
			})
			req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/projects", bytes.NewReader(body))
			req.Header.Set("Authorization", "Bearer "+userAuth.AccessToken)
			req.Header.Set("Content-Type", "application/json")
			resp, err := ts.Client().Do(req)
			if err != nil {
				t.Fatalf("project %d creation failed: %v", i, err)
			}
			resp.Body.Close()
			if resp.StatusCode != http.StatusCreated {
				t.Fatalf("expected 201 Created for project %d, got %d", i, resp.StatusCode)
			}
		}

		// 6th project creation must return 403 or 429
		body, _ := json.Marshal(map[string]string{
			"name": "Over Limit Project",
		})
		req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/projects", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+userAuth.AccessToken)
		req.Header.Set("Content-Type", "application/json")
		resp, err := ts.Client().Do(req)
		if err != nil {
			t.Fatalf("6th project creation failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusTooManyRequests {
			t.Errorf("expected 403 Forbidden or 429 Too Many Requests for quota exceeded, got %d", resp.StatusCode)
		}
	})

	t.Run("Concurrent creation race: 20 goroutines with limit 5 -> exactly 5 succeed", func(t *testing.T) {
		raceUser, err := svc.Register(ctx, domain.RegisterInput{
			Email:       "racequota@example.com",
			DisplayName: "Race Quota User",
			Password:    "Password123!",
		})
		if err != nil {
			t.Fatalf("failed to register race user: %v", err)
		}

		const totalGoroutines = 20
		var successCount int32
		var rejectedCount int32

		var wg sync.WaitGroup
		startSignal := make(chan struct{})

		for i := 0; i < totalGoroutines; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				<-startSignal

				body, _ := json.Marshal(map[string]string{
					"name": fmt.Sprintf("Concurrent Project %d", idx),
				})
				req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/projects", bytes.NewReader(body))
				req.Header.Set("Authorization", "Bearer "+raceUser.AccessToken)
				req.Header.Set("Content-Type", "application/json")

				resp, err := ts.Client().Do(req)
				if err != nil {
					return
				}
				defer resp.Body.Close()

				if resp.StatusCode == http.StatusCreated {
					atomic.AddInt32(&successCount, 1)
				} else if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
					atomic.AddInt32(&rejectedCount, 1)
				}
			}(i)
		}

		close(startSignal) // Trigger concurrent execution
		wg.Wait()

		if successCount != 5 {
			t.Errorf("expected exactly 5 successful project creations, got %d", successCount)
		}
		if rejectedCount != 15 {
			t.Errorf("expected 15 requests rejected with 403/429, got %d", rejectedCount)
		}
	})
}
