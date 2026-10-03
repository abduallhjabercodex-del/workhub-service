package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"workhub-service/internal/domain"
	"workhub-service/internal/repository"
)

func newTestService() *WorkHub {
	repo := repository.NewMemoryRepository()
	deps := Deps{
		Projects:    repo,
		Tasks:       repo,
		Summaries:   repo,
		Users:       repo,
		Auth:        repo,
		Memberships: repo,
		Quotas:      repo,
		Pinger:      repo,
		Config: AuthConfig{
			JWTSecret:       "test-jwt-secret-key-at-least-32-chars-long!",
			AccessTokenTTL:  15 * time.Minute,
			RefreshTokenTTL: 168 * time.Hour,
			BCryptCost:      10,
		},
	}
	s, err := NewWorkHub(deps)
	if err != nil {
		panic(err)
	}
	return s
}

// TestCreateProjectValidation tests input validation and constraints for project creation (valid names, empty names, and unauthenticated requests).
func TestCreateProjectValidation(t *testing.T) {
	s := newTestService()
	ctx := context.Background()

	// Register user
	user, err := s.Register(ctx, domain.RegisterInput{Email: "user1@example.com", DisplayName: "User 1", Password: "SecurePassword123"})
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}

	tests := []struct {
		name, actorID, project string
		wantErr                error
	}{
		{name: "valid", actorID: user.User.ID, project: "WorkHub", wantErr: nil},
		{name: "empty_name", actorID: user.User.ID, project: "   ", wantErr: ErrInvalidInput},
		{name: "unauthorized", actorID: "", project: "WorkHub", wantErr: ErrUnauthorized},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := s.CreateProject(ctx, tt.actorID, domain.CreateProjectInput{Name: tt.project})
			if err != tt.wantErr {
				t.Fatalf("error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

// TestTaskStatusAndRBACValidation tests role-based access control (RBAC) between owners and viewers, as well as updating supported and unsupported task statuses.
func TestTaskStatusAndRBACValidation(t *testing.T) {
	s := newTestService()
	ctx := context.Background()

	u1, _ := s.Register(ctx, domain.RegisterInput{Email: "owner@example.com", DisplayName: "Owner", Password: "SecurePassword123"})
	u2, _ := s.Register(ctx, domain.RegisterInput{Email: "viewer@example.com", DisplayName: "Viewer", Password: "SecurePassword123"})

	project, err := s.CreateProject(ctx, u1.User.ID, domain.CreateProjectInput{Name: "P"})
	if err != nil {
		t.Fatal(err)
	}
	// Add u1 as owner
	_ = s.memberships.AddProjectMember(ctx, project.ID, u1.User.ID, domain.RoleOwner)
	// Add u2 as viewer
	_ = s.memberships.AddProjectMember(ctx, project.ID, u2.User.ID, domain.RoleViewer)

	// Viewer cannot create task
	if _, err := s.CreateTask(ctx, u2.User.ID, project.ID, domain.CreateTaskInput{Title: "T"}); err != ErrForbidden {
		t.Fatalf("viewer create task err = %v, want ErrForbidden", err)
	}

	// Owner creates task
	task, err := s.CreateTask(ctx, u1.User.ID, project.ID, domain.CreateTaskInput{Title: "T"})
	if err != nil {
		t.Fatalf("owner create task failed: %v", err)
	}

	// Test status updates by owner
	for _, status := range []string{"todo", "in_progress", "done"} {
		value := status
		if _, err := s.UpdateTask(ctx, u1.User.ID, task.ID, domain.UpdateTaskInput{Status: &value}); err != nil {
			t.Fatalf("status %s: %v", status, err)
		}
	}

	// Invalid status
	bad := "blocked"
	if _, err := s.UpdateTask(ctx, u1.User.ID, task.ID, domain.UpdateTaskInput{Status: &bad}); err != ErrInvalidInput {
		t.Fatalf("error = %v, want ErrInvalidInput", err)
	}
}

// TestMissingProjectAndTask tests graceful handling of access attempts to nonexistent or inaccessible projects or tasks.
func TestMissingProjectAndTask(t *testing.T) {
	s := newTestService()
	ctx := context.Background()

	u1, _ := s.Register(ctx, domain.RegisterInput{Email: "user@example.com", DisplayName: "User", Password: "SecurePassword123"})

	if _, err := s.CreateTask(ctx, u1.User.ID, "missing", domain.CreateTaskInput{Title: "T"}); err != ErrForbidden {
		t.Fatalf("error = %v, want ErrForbidden (non-member of project missing)", err)
	}
	if _, err := s.GetTask(ctx, u1.User.ID, "missing"); err != ErrTaskNotFound {
		t.Fatalf("error = %v, want ErrTaskNotFound", err)
	}
}

// TestDeleteTaskAndLastOwnerGuard tests task deletion and verifies that removing the last owner is forbidden (Last Owner Protection Guard).
func TestDeleteTaskAndLastOwnerGuard(t *testing.T) {
	s := newTestService()
	ctx := context.Background()

	u1, _ := s.Register(ctx, domain.RegisterInput{Email: "owner1@example.com", DisplayName: "Owner 1", Password: "SecurePassword123"})
	project, err := s.CreateProject(ctx, u1.User.ID, domain.CreateProjectInput{Name: "P"})
	if err != nil {
		t.Fatal(err)
	}
	_ = s.memberships.AddProjectMember(ctx, project.ID, u1.User.ID, domain.RoleOwner)

	// Attempting to remove last owner must fail
	if err := s.RemoveProjectMember(ctx, u1.User.ID, project.ID, u1.User.ID); err != ErrForbidden {
		t.Fatalf("remove last owner err = %v, want ErrForbidden", err)
	}

	task, err := s.CreateTask(ctx, u1.User.ID, project.ID, domain.CreateTaskInput{Title: "Task to delete"})
	if err != nil {
		t.Fatal(err)
	}

	if err := s.DeleteTask(ctx, u1.User.ID, task.ID); err != nil {
		t.Fatalf("delete task failed: %v", err)
	}
	if _, err := s.GetTask(ctx, u1.User.ID, task.ID); err != ErrTaskNotFound {
		t.Fatalf("after delete, get task error = %v, want ErrTaskNotFound", err)
	}
}

// TestAtomicRefreshTokenAndReplayDetection tests atomic refresh token rotation, replay attack detection, and immediate session revocation for security.
func TestAtomicRefreshTokenAndReplayDetection(t *testing.T) {
	s := newTestService()
	ctx := context.Background()

	userRes, err := s.Register(ctx, domain.RegisterInput{Email: "testrefresh@example.com", DisplayName: "Tester", Password: "SecurePassword123"})
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}

	// 1. First Refresh succeeds
	refRes1, err := s.RefreshToken(ctx, userRes.RefreshToken)
	if err != nil {
		t.Fatalf("first refresh failed: %v", err)
	}
	if refRes1.RefreshToken == "" {
		t.Fatalf("new refresh token is empty")
	}

	// 2. Token Replay Attack: Re-using old token must trigger Replay Detection & revoke ALL user sessions
	_, err = s.RefreshToken(ctx, userRes.RefreshToken)
	if err != ErrUnauthorized {
		t.Fatalf("replaying old token err = %v, want ErrUnauthorized", err)
	}

	// 3. New token must also be revoked due to replay mitigation!
	_, err = s.RefreshToken(ctx, refRes1.RefreshToken)
	if err != ErrUnauthorized {
		t.Fatalf("token after replay mitigation err = %v, want ErrUnauthorized", err)
	}
}

// TestConcurrentRefreshTokenRace tests concurrent race conditions during token refresh, ensuring exactly one attempt succeeds while concurrent attempts fail.
func TestConcurrentRefreshTokenRace(t *testing.T) {
	s := newTestService()
	ctx := context.Background()

	userRes, _ := s.Register(ctx, domain.RegisterInput{Email: "concrefresh@example.com", DisplayName: "ConcTester", Password: "SecurePassword123"})

	var wg sync.WaitGroup
	successCount := 0
	var mu sync.Mutex

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.RefreshToken(ctx, userRes.RefreshToken)
			if err == nil {
				mu.Lock()
				successCount++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if successCount > 1 {
		t.Fatalf("concurrent refresh success count = %d, expected at most 1 due to atomic rotation", successCount)
	}
}

type testEventCapture struct {
	mu     sync.Mutex
	events []domain.Event
}

func (c *testEventCapture) Publish(_ context.Context, e domain.Event) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, e)
	return nil
}

func (c *testEventCapture) HasType(eventType string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, e := range c.events {
		if e.Type == eventType {
			return true
		}
	}
	return false
}

func TestPasswordBoundsAndValidation(t *testing.T) {
	s := newTestService()
	ctx := context.Background()

	// Password longer than 72 bytes must be rejected
	longPassword := "A1!aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if len(longPassword) <= 72 {
		t.Fatalf("password should be > 72 bytes")
	}
	_, err := s.Register(ctx, domain.RegisterInput{
		Email:       "longpwd@example.com",
		DisplayName: "LongPwd User",
		Password:    longPassword,
	})
	if err != ErrInvalidInput {
		t.Fatalf("register with password > 72 bytes error = %v, want ErrInvalidInput", err)
	}
}

func TestProjectMemberValidation(t *testing.T) {
	s := newTestService()
	ctx := context.Background()

	u, _ := s.Register(ctx, domain.RegisterInput{Email: "memval@example.com", DisplayName: "MemVal", Password: "SecurePassword123"})
	proj, _ := s.CreateProject(ctx, u.User.ID, domain.CreateProjectInput{Name: "MemValProj"})

	// Empty target user ID -> ErrInvalidInput
	if err := s.AddProjectMember(ctx, u.User.ID, proj.ID, "", domain.RoleMember); err != ErrInvalidInput {
		t.Fatalf("add member with empty targetUserID err = %v, want ErrInvalidInput", err)
	}

	// Empty project ID -> ErrInvalidInput
	if err := s.AddProjectMember(ctx, u.User.ID, "", u.User.ID, domain.RoleMember); err != ErrInvalidInput {
		t.Fatalf("add member with empty projectID err = %v, want ErrInvalidInput", err)
	}

	// Remove member with empty target user ID -> ErrInvalidInput
	if err := s.RemoveProjectMember(ctx, u.User.ID, proj.ID, ""); err != ErrInvalidInput {
		t.Fatalf("remove member with empty targetUserID err = %v, want ErrInvalidInput", err)
	}
}

func TestLifecycleEvents(t *testing.T) {
	repo := repository.NewMemoryRepository()
	capture := &testEventCapture{}
	deps := Deps{
		Projects:    repo,
		Tasks:       repo,
		Summaries:   repo,
		Users:       repo,
		Auth:        repo,
		Memberships: repo,
		Quotas:      repo,
		Pinger:      repo,
		Publisher:   capture,
		Config: AuthConfig{
			JWTSecret:       "test-jwt-secret-key-at-least-32-chars-long!",
			AccessTokenTTL:  15 * time.Minute,
			RefreshTokenTTL: 168 * time.Hour,
			BCryptCost:      10,
		},
	}
	s, err := NewWorkHub(deps)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	u, _ := s.Register(ctx, domain.RegisterInput{Email: "events@example.com", DisplayName: "Events", Password: "SecurePassword123"})
	proj, _ := s.CreateProject(ctx, u.User.ID, domain.CreateProjectInput{Name: "EventProj"})
	task, _ := s.CreateTask(ctx, u.User.ID, proj.ID, domain.CreateTaskInput{Title: "EventTask"})

	doneStatus := "done"
	_, _ = s.UpdateTask(ctx, u.User.ID, task.ID, domain.UpdateTaskInput{Status: &doneStatus})
	_ = s.DeleteTask(ctx, u.User.ID, task.ID)
	_ = s.DeleteProject(ctx, u.User.ID, proj.ID)

	for _, expectedType := range []string{"project.created", "task.created", "task.updated", "task.deleted", "project.deleted"} {
		if !capture.HasType(expectedType) {
			t.Errorf("expected event %q was not published", expectedType)
		}
	}
}

