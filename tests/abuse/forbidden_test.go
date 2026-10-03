package abuse_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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

func setupAbuseApp(t *testing.T) (http.Handler, *service.WorkHub, string) {
	t.Helper()
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
		t.Fatalf("failed to create WorkHub service: %v", err)
	}

	api := handler.NewAPI(svc, nil, nil, slog.Default())
	authHandler := middleware.Authenticate(secret)(api.Routes())
	finalHandler := middleware.SecurityHeaders("")(authHandler)

	return finalHandler, svc, secret
}

// TestForbiddenAccess tests all 403 Forbidden scenarios listed in Day 15 requirements.
func TestForbiddenAccess(t *testing.T) {
	handlerStack, svc, _ := setupAbuseApp(t)
	ts := httptest.NewServer(handlerStack)
	defer ts.Close()

	ctx := context.Background()

	// 1. Register Owner
	owner, err := svc.Register(ctx, domain.RegisterInput{
		Email:       "owner@example.com",
		DisplayName: "Owner User",
		Password:    "Password123!",
	})
	if err != nil {
		t.Fatalf("failed to register owner: %v", err)
	}

	// 2. Register Admin
	admin, err := svc.Register(ctx, domain.RegisterInput{
		Email:       "admin@example.com",
		DisplayName: "Admin User",
		Password:    "Password123!",
	})
	if err != nil {
		t.Fatalf("failed to register admin: %v", err)
	}

	// 3. Register Member
	member, err := svc.Register(ctx, domain.RegisterInput{
		Email:       "member@example.com",
		DisplayName: "Member User",
		Password:    "Password123!",
	})
	if err != nil {
		t.Fatalf("failed to register member: %v", err)
	}

	// 4. Register Viewer
	viewer, err := svc.Register(ctx, domain.RegisterInput{
		Email:       "viewer@example.com",
		DisplayName: "Viewer User",
		Password:    "Password123!",
	})
	if err != nil {
		t.Fatalf("failed to register viewer: %v", err)
	}

	// 5. Register Non-Member
	nonMember, err := svc.Register(ctx, domain.RegisterInput{
		Email:       "nonmember@example.com",
		DisplayName: "Non Member",
		Password:    "Password123!",
	})
	if err != nil {
		t.Fatalf("failed to register non-member: %v", err)
	}

	// Create project as owner
	proj, err := svc.CreateProject(ctx, owner.User.ID, domain.CreateProjectInput{
		Name: "Target Project",
	})
	if err != nil {
		t.Fatalf("failed to create project: %v", err)
	}

	// Add admin, member, viewer to project
	_ = svc.AddProjectMember(ctx, owner.User.ID, proj.ID, admin.User.ID, domain.RoleAdmin)
	_ = svc.AddProjectMember(ctx, owner.User.ID, proj.ID, member.User.ID, domain.RoleMember)
	_ = svc.AddProjectMember(ctx, owner.User.ID, proj.ID, viewer.User.ID, domain.RoleViewer)

	t.Run("Member tries to delete project -> 403", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/api/v1/projects/%s", ts.URL, proj.ID), nil)
		req.Header.Set("Authorization", "Bearer "+member.AccessToken)
		resp, err := ts.Client().Do(req)
		if err != nil {
			t.Fatalf("http request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("expected 403 Forbidden, got %d", resp.StatusCode)
		}
	})

	t.Run("Viewer tries to create task -> 403", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"title": "Unauthorized Task",
		})
		req, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/api/v1/projects/%s/tasks", ts.URL, proj.ID), bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+viewer.AccessToken)
		req.Header.Set("Content-Type", "application/json")
		resp, err := ts.Client().Do(req)
		if err != nil {
			t.Fatalf("http request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("expected 403 Forbidden, got %d", resp.StatusCode)
		}
	})

	t.Run("Admin tries to remove owner -> 403", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/api/v1/projects/%s/members/%s", ts.URL, proj.ID, owner.User.ID), nil)
		req.Header.Set("Authorization", "Bearer "+admin.AccessToken)
		resp, err := ts.Client().Do(req)
		if err != nil {
			t.Fatalf("http request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("expected 403 Forbidden, got %d", resp.StatusCode)
		}
	})

	t.Run("Non-member tries to read project -> 403", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/api/v1/projects/%s", ts.URL, proj.ID), nil)
		req.Header.Set("Authorization", "Bearer "+nonMember.AccessToken)
		resp, err := ts.Client().Do(req)
		if err != nil {
			t.Fatalf("http request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("expected 403 Forbidden, got %d", resp.StatusCode)
		}
	})

	t.Run("Non-admin user tries to modify quota -> 403", func(t *testing.T) {
		body, _ := json.Marshal(map[string]int{
			"max_projects": 100,
		})
		req, _ := http.NewRequest(http.MethodPut, fmt.Sprintf("%s/api/v1/quotas/%s", ts.URL, member.User.ID), bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+member.AccessToken)
		req.Header.Set("Content-Type", "application/json")
		resp, err := ts.Client().Do(req)
		if err != nil {
			t.Fatalf("http request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusForbidden {
			bodyBytes, _ := io.ReadAll(resp.Body)
			t.Errorf("expected 403 Forbidden, got %d: %s", resp.StatusCode, string(bodyBytes))
		}
	})
}
