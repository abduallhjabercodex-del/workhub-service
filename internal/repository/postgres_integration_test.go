package repository

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"workhub-service/internal/domain"
)

func integrationPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("WORKHUB_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("WORKHUB_TEST_DATABASE_URL is not set")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("ping database: %v", err)
	}
	return pool
}

func TestPostgresRepositoryCreateTaskWithAuditCommitsAtomically(t *testing.T) {
	pool := integrationPool(t)
	defer pool.Close()
	repo := NewPostgresRepository(pool, 1)
	id := strconv.FormatInt(time.Now().UnixNano(), 10)
	task := domain.Task{ID: id, ProjectID: "1", Title: "integration commit", Status: "todo", CreatedAt: time.Now().UTC()}
	created, err := repo.CreateTaskWithAudit(context.Background(), task, domain.AuditEvent{
		ProjectID: "1", TaskID: id, EventType: "task.created", Payload: []byte(`{"source":"integration"}`), CreatedAt: task.CreatedAt,
	})
	if err != nil {
		t.Fatalf("CreateTaskWithAudit() error = %v", err)
	}
	var tasks, audits int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM tasks WHERE id = $1", created.ID).Scan(&tasks); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM audit_events WHERE task_id = $1", created.ID).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if tasks != 1 || audits != 1 {
		t.Fatalf("commit counts = tasks:%d audits:%d, want 1/1", tasks, audits)
	}
	_, _ = pool.Exec(context.Background(), "DELETE FROM audit_events WHERE task_id = $1", created.ID)
	_, _ = pool.Exec(context.Background(), "DELETE FROM tasks WHERE id = $1", created.ID)
}

func TestPostgresRepositoryCreateTaskWithAuditRollsBackOnAuditFailure(t *testing.T) {
	pool := integrationPool(t)
	defer pool.Close()
	repo := NewPostgresRepository(pool, 1)
	id := strconv.FormatInt(time.Now().UnixNano()+1, 10)
	task := domain.Task{ID: id, ProjectID: "1", Title: "integration rollback", Status: "todo", CreatedAt: time.Now().UTC()}
	_, err := repo.CreateTaskWithAudit(context.Background(), task, domain.AuditEvent{
		ProjectID: "1", TaskID: id, EventType: "", Payload: []byte(`{}`), CreatedAt: task.CreatedAt,
	})
	if !errors.Is(err, ErrConstraint) {
		t.Fatalf("error = %v, want ErrConstraint", err)
	}
	var tasks int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM tasks WHERE id = $1", id).Scan(&tasks); err != nil {
		t.Fatal(err)
	}
	if tasks != 0 {
		t.Fatalf("rollback left %d task rows, want 0", tasks)
	}
}

func TestPostgresRepositoryMapsNotFoundAndTimeout(t *testing.T) {
	pool := integrationPool(t)
	defer pool.Close()
	repo := NewPostgresRepository(pool, 1)
	if _, err := repo.GetTask(context.Background(), "9223372036854770000"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("not found error = %v, want ErrNotFound", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	_, err := repo.GetProject(ctx, "1")
	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		t.Fatalf("timeout error = %v, want context deadline/canceled", err)
	}
}

func TestPostgresRepositoryMigrationFilesExist(t *testing.T) {
	for _, name := range []string{"001_init.up.sql", "001_init.down.sql"} {
		path := filepath.Join("..", "..", "db", "migrations", name)
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("migration %s: %v", name, err)
		}
	}
}
