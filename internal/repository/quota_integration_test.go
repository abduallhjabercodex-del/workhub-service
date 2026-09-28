package repository

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"workhub-service/internal/domain"

	"github.com/jackc/pgx/v5/pgxpool"
)

func getTestPostgresRepo(t *testing.T) *PostgresRepository {
	t.Helper()
	url := os.Getenv("WORKHUB_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("WORKHUB_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("pgxpool.New() error = %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("ping database: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return NewPostgresRepository(pool)
}

// TestPostgresRepositoryAtomicProjectQuotaWithAdvisoryLocks يختبر منع إغراق وإنشاء المشاريع المتزامنة بضمان الذرية عبر أقفال PostgreSQL الاستشارية (Advisory Locks) لعدم تجاوز سقف المشاريع.
func TestPostgresRepositoryAtomicProjectQuotaWithAdvisoryLocks(t *testing.T) {
	repo := getTestPostgresRepo(t)
	ctx := context.Background()

	// Register a dedicated test owner user
	userEmail := fmt.Sprintf("quota_owner_%d@example.com", time.Now().UnixNano())
	u, err := repo.CreateUser(ctx, userEmail, "Quota Owner", "hash123")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	maxProjects := 3
	numGoroutines := 10

	var wg sync.WaitGroup
	var successCount int64
	var quotaErrCount int64

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			p := domain.Project{
				OwnerID:   u.ID,
				Name:      fmt.Sprintf("Concurrent Proj %d", idx),
				CreatedAt: time.Now().UTC(),
			}
			_, err := repo.CreateProjectWithQuota(ctx, p, maxProjects)
			if err == nil {
				atomic.AddInt64(&successCount, 1)
			} else {
				var qErr *domain.QuotaError
				if errors.As(err, &qErr) || errors.Is(err, domain.ErrQuotaExceeded) {
					atomic.AddInt64(&quotaErrCount, 1)
				} else {
					t.Errorf("Unexpected error (%T): %v", err, err)
				}
			}
		}(i)
	}

	wg.Wait()

	if successCount != int64(maxProjects) {
		t.Errorf("Expected exactly %d successful project creations, got %d", maxProjects, successCount)
	}
	if quotaErrCount != int64(numGoroutines-maxProjects) {
		t.Errorf("Expected %d quota exceeded errors, got %d", numGoroutines-maxProjects, quotaErrCount)
	}

	totalCreated, err := repo.CountUserProjects(ctx, u.ID)
	if err != nil {
		t.Fatalf("CountUserProjects failed: %v", err)
	}
	if totalCreated != maxProjects {
		t.Errorf("CountUserProjects expected %d, got %d", maxProjects, totalCreated)
	}
}

// TestPostgresRepositoryAtomicTaskQuotaWithAdvisoryLocks يختبر التحكم الذري وحماية حصة المهام للمشروع أثناء الإنشاء المتزامن باستخدام الأقفال الاستشارية (Advisory Locks).
func TestPostgresRepositoryAtomicTaskQuotaWithAdvisoryLocks(t *testing.T) {
	repo := getTestPostgresRepo(t)
	ctx := context.Background()

	// Create a test project
	projName := fmt.Sprintf("Quota Task Proj %d", time.Now().UnixNano())
	p, err := repo.CreateProject(ctx, domain.Project{Name: projName, CreatedAt: time.Now().UTC()})
	if err != nil {
		t.Fatalf("CreateProject failed: %v", err)
	}

	maxTasks := 4
	numGoroutines := 12

	var wg sync.WaitGroup
	var successCount int64
	var quotaErrCount int64

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			task := domain.Task{
				ProjectID: p.ID,
				Title:     fmt.Sprintf("Concurrent Task %d", idx),
				Status:    "todo",
				CreatedAt: time.Now().UTC(),
			}
			audit := domain.AuditEvent{
				ProjectID: p.ID,
				EventType: "task.created",
				Payload:   []byte(`{"test":true}`),
				CreatedAt: task.CreatedAt,
			}
			_, err := repo.CreateTaskWithQuota(ctx, task, audit, maxTasks)
			if err == nil {
				atomic.AddInt64(&successCount, 1)
			} else {
				var qErr *domain.QuotaError
				if errors.As(err, &qErr) || errors.Is(err, domain.ErrQuotaExceeded) {
					atomic.AddInt64(&quotaErrCount, 1)
				} else {
					t.Errorf("Unexpected error in CreateTaskWithQuota: %v", err)
				}
			}
		}(i)
	}

	wg.Wait()

	if successCount != int64(maxTasks) {
		t.Errorf("Expected exactly %d successful task creations, got %d", maxTasks, successCount)
	}
	if quotaErrCount != int64(numGoroutines-maxTasks) {
		t.Errorf("Expected %d quota exceeded errors, got %d", numGoroutines-maxTasks, quotaErrCount)
	}

	totalCreated, err := repo.CountProjectTasks(ctx, p.ID)
	if err != nil {
		t.Fatalf("CountProjectTasks failed: %v", err)
	}
	if totalCreated != maxTasks {
		t.Errorf("CountProjectTasks expected %d, got %d", maxTasks, totalCreated)
	}
}

// TestPostgresRepositoryUserQuotaAdminOverrides يختبر قراءة وتعديل استثناءات حصص المستخدمين (Quota Overrides) المخصصة بواسطة مسؤولي النظام.
func TestPostgresRepositoryUserQuotaAdminOverrides(t *testing.T) {
	repo := getTestPostgresRepo(t)
	ctx := context.Background()

	userEmail := fmt.Sprintf("quota_admin_%d@example.com", time.Now().UnixNano())
	u, err := repo.CreateUser(ctx, userEmail, "Admin Quota Test", "pass123")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	// 1. Get default quota (should be empty override struct with UserID)
	initial, _, err := repo.GetUserQuota(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetUserQuota failed: %v", err)
	}
	if initial.UserID != u.ID || initial.MaxProjects != nil {
		t.Errorf("Expected unconfigured quota for user, got %+v", initial)
	}

	// 2. Set custom quota override
	customMaxProjects := 15
	customMaxMembers := 25
	customMaxTasks := 500

	override := domain.UserQuota{
		UserID:               u.ID,
		MaxProjects:          &customMaxProjects,
		MaxMembersPerProject: &customMaxMembers,
		MaxTasksPerProject:   &customMaxTasks,
	}

	if err := repo.SetUserQuota(ctx, override); err != nil {
		t.Fatalf("SetUserQuota failed: %v", err)
	}

	// 3. Verify retrieved override
	fetched, _, err := repo.GetUserQuota(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetUserQuota after set failed: %v", err)
	}
	if fetched.MaxProjects == nil || *fetched.MaxProjects != 15 {
		t.Errorf("MaxProjects expected 15, got %v", fetched.MaxProjects)
	}
	if fetched.MaxMembersPerProject == nil || *fetched.MaxMembersPerProject != 25 {
		t.Errorf("MaxMembersPerProject expected 25, got %v", fetched.MaxMembersPerProject)
	}
	if fetched.MaxTasksPerProject == nil || *fetched.MaxTasksPerProject != 500 {
		t.Errorf("MaxTasksPerProject expected 500, got %v", fetched.MaxTasksPerProject)
	}
}
