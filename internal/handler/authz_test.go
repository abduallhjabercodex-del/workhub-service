package handler

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"workhub-service/internal/domain"
	"workhub-service/internal/middleware"
	"workhub-service/internal/repository"
	"workhub-service/internal/service"
)

func newTestSecurityAPI() (*API, *service.WorkHub, domain.AuthResponse, domain.AuthResponse) {
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

	// Create User A (Owner)
	u1, _ := s.Register(nil, domain.RegisterInput{Email: "usera@example.com", DisplayName: "User A", Password: "SecurePassword123"})
	// Create User B (Attacker / Non-member)
	u2, _ := s.Register(nil, domain.RegisterInput{Email: "userb@example.com", DisplayName: "User B", Password: "SecurePassword123"})

	return NewAPI(s, nil, nil, slog.Default()), s, u1, u2
}

// TestSecurityHeadersOnAPIRoutes tests that defense-in-depth security headers are applied to secure all API routes.
func TestSecurityHeadersOnAPIRoutes(t *testing.T) {
	api, _, _, _ := newTestSecurityAPI()
	handlerStack := middleware.SecurityHeaders("")(api.Routes())

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	handlerStack.ServeHTTP(rec, req)

	if rec.Header().Get("X-Frame-Options") != "DENY" {
		t.Errorf("X-Frame-Options missing or invalid")
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("X-Content-Type-Options missing or invalid")
	}
}

// TestHorizontalAndVerticalPrivilegeEscalation tests prevention of horizontal privilege escalation (non-members accessing projects) and vertical privilege escalation (regular members promoting themselves or others without owner privileges).
func TestHorizontalAndVerticalPrivilegeEscalation(t *testing.T) {
	api, s, u1, u2 := newTestSecurityAPI()
	tokenA := u1.AccessToken
	tokenB := u2.AccessToken
	handlerStack := middleware.Authenticate("test-jwt-secret-key-at-least-32-chars-long!")(api.Routes())

	// User A creates Project 1
	p1Body := `{"name":"Project 1"}`
	reqP1 := httptest.NewRequest(http.MethodPost, "/api/v1/projects", bytes.NewBufferString(p1Body))
	reqP1.Header.Set("Content-Type", "application/json")
	reqP1.Header.Set("Authorization", "Bearer "+tokenA)
	recP1 := httptest.NewRecorder()
	handlerStack.ServeHTTP(recP1, reqP1)

	if recP1.Code != http.StatusCreated {
		t.Fatalf("create project status = %d, want 201", recP1.Code)
	}

	var p1Res struct {
		Data domain.Project `json:"data"`
	}
	_ = json.Unmarshal(recP1.Body.Bytes(), &p1Res)
	p1ID := p1Res.Data.ID

	// User B (Non-member) tries to list members of Project 1 -> 403 Forbidden (Horizontal Escalation Attack)
	reqBMembers := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+p1ID+"/members", nil)
	reqBMembers.Header.Set("Authorization", "Bearer "+tokenB)
	recBMembers := httptest.NewRecorder()
	handlerStack.ServeHTTP(recBMembers, reqBMembers)

	if recBMembers.Code != http.StatusForbidden {
		t.Fatalf("non-member access status = %d, want 403 Forbidden", recBMembers.Code)
	}

	// User A adds User C as a regular "member"
	u3, _ := s.Register(nil, domain.RegisterInput{Email: "userc@example.com", DisplayName: "User C", Password: "SecurePassword123"})
	tokenC := u3.AccessToken

	addMemberBody := `{"userId":"` + u3.User.ID + `","role":"member"}`
	reqAdd := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+p1ID+"/members", bytes.NewBufferString(addMemberBody))
	reqAdd.Header.Set("Content-Type", "application/json")
	reqAdd.Header.Set("Authorization", "Bearer "+tokenA)
	recAdd := httptest.NewRecorder()
	handlerStack.ServeHTTP(recAdd, reqAdd)

	if recAdd.Code != http.StatusCreated {
		t.Fatalf("add member status = %d, want 201", recAdd.Code)
	}

	// User C (Regular Member) tries to add another user as Owner/Admin -> 403 Forbidden (Vertical Escalation Attack)
	promoteBody := `{"userId":"` + u2.User.ID + `","role":"admin"}`
	reqPromote := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+p1ID+"/members", bytes.NewBufferString(promoteBody))
	reqPromote.Header.Set("Content-Type", "application/json")
	reqPromote.Header.Set("Authorization", "Bearer "+tokenC)
	recPromote := httptest.NewRecorder()
	handlerStack.ServeHTTP(recPromote, reqPromote)

	if recPromote.Code != http.StatusForbidden {
		t.Fatalf("vertical privilege escalation status = %d, want 403 Forbidden", recPromote.Code)
	}
}

// TestPayloadSizeLimiter tests blocking oversized payloads to protect the server against memory exhaustion and Denial of Service attempts.
func TestPayloadSizeLimiter(t *testing.T) {
	api, _, u1, _ := newTestSecurityAPI()
	tokenA := u1.AccessToken
	handlerStack := middleware.Authenticate("test-jwt-secret-key-at-least-32-chars-long!")(api.Routes())

	// Create oversized payload (> 1MB)
	largeName := strings.Repeat("A", 1024*1024+100)
	largeBody := `{"name":"` + largeName + `"}`

	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects", bytes.NewBufferString(largeBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tokenA)
	rec := httptest.NewRecorder()

	handlerStack.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest && rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized body status = %d, want 400 or 413", rec.Code)
	}
}
