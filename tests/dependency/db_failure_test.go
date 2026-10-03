package dependency_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"workhub-service/internal/domain"
	"workhub-service/internal/handler"
	"workhub-service/internal/infrastructure/auth"
	"workhub-service/internal/middleware"
	"workhub-service/internal/service"
)

// failingRepository implements all Repository interfaces to simulate PostgreSQL connection refusal / timeouts.
type failingRepository struct{}

func (f *failingRepository) Ping(_ context.Context) error {
	return errors.New("connection refused to postgres:5432")
}

func (f *failingRepository) CreateProject(_ context.Context, _ domain.Project) (domain.Project, error) {
	return domain.Project{}, errors.New("connection refused")
}

func (f *failingRepository) CreateProjectWithQuota(_ context.Context, _ domain.Project, _ int) (domain.Project, error) {
	return domain.Project{}, errors.New("connection refused")
}

func (f *failingRepository) CountUserProjects(_ context.Context, _ string) (int, error) {
	return 0, errors.New("connection refused")
}

func (f *failingRepository) ListProjects(_ context.Context, _, _, _ string, _, _ int) ([]domain.Project, error) {
	return nil, errors.New("connection refused")
}

func (f *failingRepository) GetProject(_ context.Context, _ string) (domain.Project, error) {
	return domain.Project{}, errors.New("connection refused")
}

func (f *failingRepository) DeleteProject(_ context.Context, _ string) error {
	return errors.New("connection refused")
}

func (f *failingRepository) GetProjectSummary(_ context.Context, _ string) (domain.ProjectSummary, error) {
	return domain.ProjectSummary{}, errors.New("connection refused")
}

func (f *failingRepository) CreateTask(_ context.Context, _ domain.Task) (domain.Task, error) {
	return domain.Task{}, errors.New("connection refused")
}

func (f *failingRepository) CreateTaskWithQuota(_ context.Context, _ domain.Task, _ domain.AuditEvent, _ int) (domain.Task, error) {
	return domain.Task{}, errors.New("connection refused")
}

func (f *failingRepository) CountProjectTasks(_ context.Context, _ string) (int, error) {
	return 0, errors.New("connection refused")
}

func (f *failingRepository) ListTasks(_ context.Context, _, _, _, _ string, _, _ int) ([]domain.Task, int, error) {
	return nil, 0, errors.New("connection refused")
}

func (f *failingRepository) GetTask(_ context.Context, _ string) (domain.Task, error) {
	return domain.Task{}, errors.New("connection refused")
}

func (f *failingRepository) UpdateTask(_ context.Context, _ domain.Task) (domain.Task, error) {
	return domain.Task{}, errors.New("connection refused")
}

func (f *failingRepository) DeleteTask(_ context.Context, _ string) error {
	return errors.New("connection refused")
}

func (f *failingRepository) CreateUser(_ context.Context, _, _, _ string) (domain.User, error) {
	return domain.User{}, errors.New("connection refused")
}

func (f *failingRepository) GetUserByEmail(_ context.Context, _ string) (domain.User, error) {
	return domain.User{}, errors.New("connection refused")
}

func (f *failingRepository) GetUserByID(_ context.Context, _ string) (domain.User, error) {
	return domain.User{}, errors.New("connection refused")
}

func (f *failingRepository) SaveRefreshToken(_ context.Context, _ domain.RefreshTokenSession) error {
	return errors.New("connection refused")
}

func (f *failingRepository) GetRefreshToken(_ context.Context, _ string) (domain.RefreshTokenSession, error) {
	return domain.RefreshTokenSession{}, errors.New("connection refused")
}

func (f *failingRepository) RevokeRefreshToken(_ context.Context, _ string, _ *string) error {
	return errors.New("connection refused")
}

func (f *failingRepository) RotateRefreshToken(_ context.Context, _ string, _ domain.RefreshTokenSession) (domain.RefreshTokenSession, error) {
	return domain.RefreshTokenSession{}, errors.New("connection refused")
}

func (f *failingRepository) RevokeAllUserRefreshTokens(_ context.Context, _ string) error {
	return errors.New("connection refused")
}

func (f *failingRepository) AddProjectMember(_ context.Context, _, _ string, _ domain.Role) error {
	return errors.New("connection refused")
}

func (f *failingRepository) AddProjectMemberWithQuota(_ context.Context, _, _ string, _ domain.Role, _ int) error {
	return errors.New("connection refused")
}

func (f *failingRepository) CountProjectMembers(_ context.Context, _ string) (int, error) {
	return 0, errors.New("connection refused")
}

func (f *failingRepository) GetProjectMember(_ context.Context, _, _ string) (domain.Member, error) {
	return domain.Member{}, errors.New("connection refused")
}

func (f *failingRepository) ListProjectMembers(_ context.Context, _ string) ([]domain.Member, error) {
	return nil, errors.New("connection refused")
}

func (f *failingRepository) RemoveProjectMember(_ context.Context, _, _ string) error {
	return errors.New("connection refused")
}

func (f *failingRepository) RemoveProjectMemberSafely(_ context.Context, _, _ string) error {
	return errors.New("connection refused")
}

func (f *failingRepository) GetUserQuota(_ context.Context, _ string) (domain.UserQuota, bool, error) {
	return domain.UserQuota{}, false, errors.New("connection refused")
}

func (f *failingRepository) SetUserQuota(_ context.Context, _ domain.UserQuota) error {
	return errors.New("connection refused")
}

// TestDBFailureScenarios verifies liveness, readiness, and API behavior during Database outages.
func TestDBFailureScenarios(t *testing.T) {
	failingRepo := &failingRepository{}
	secret := "test-jwt-secret-key-at-least-32-chars-long!"

	deps := service.Deps{
		Projects:             failingRepo,
		Tasks:                failingRepo,
		Summaries:            failingRepo,
		Users:                failingRepo,
		Auth:                 failingRepo,
		Memberships:          failingRepo,
		Quotas:               failingRepo,
		Pinger:               failingRepo,
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
	// Mark startup complete so readiness check evaluates DB ping
	api.AppHealth().MarkStartupComplete(true)
	api.AppHealth().MarkReady(true)

	authHandler := middleware.Authenticate(secret)(api.Routes())
	ts := httptest.NewServer(authHandler)
	defer ts.Close()

	t.Run("GET /health returns 200 (Liveness probe passes even when DB is down)", func(t *testing.T) {
		resp, err := ts.Client().Get(ts.URL + "/health")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200 OK for /health liveness probe, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /ready returns 503 (Readiness probe fails when DB Ping fails)", func(t *testing.T) {
		resp, err := ts.Client().Get(ts.URL + "/ready")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusServiceUnavailable {
			t.Errorf("expected 503 Service Unavailable for /ready probe, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /api/v1/projects returns 503 or 500 when DB query fails", func(t *testing.T) {
		// Valid JWT token to bypass auth check
		validToken, err := auth.GenerateAccessToken("1", "test@example.com", secret, time.Hour)
		if err != nil {
			t.Fatalf("failed to generate token: %v", err)
		}

		body, _ := json.Marshal(map[string]string{"name": "Failing Project"})
		req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/projects", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+validToken)
		req.Header.Set("Content-Type", "application/json")

		resp, err := ts.Client().Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusInternalServerError && resp.StatusCode != http.StatusServiceUnavailable {
			t.Errorf("expected 500 or 503 for DB failure, got %d", resp.StatusCode)
		}
	})
}
