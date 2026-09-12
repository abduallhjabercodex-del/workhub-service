package service

import (
	"context"
	"testing"

	"workhub-service/internal/domain"
	"workhub-service/internal/repository"
)

func newTestService() *WorkHub {
	repo := repository.NewMemoryRepository()
	s := NewWorkHub(repo, repo)
	s.ids = func() string { return "fixed-id" }
	return s
}

func TestCreateProjectValidation(t *testing.T) {
	s := newTestService()
	tests := []struct {
		name, project string
		wantErr       bool
	}{
		{name: "valid", project: "WorkHub"},
		{name: "empty", project: "   ", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := s.CreateProject(context.Background(), domain.CreateProjectInput{Name: tt.project})
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestTaskStatusValidation(t *testing.T) {
	s := newTestService()
	project, err := s.CreateProject(context.Background(), domain.CreateProjectInput{Name: "P"})
	if err != nil {
		t.Fatal(err)
	}
	task, err := s.CreateTask(context.Background(), project.ID, domain.CreateTaskInput{Title: "T"})
	if err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{"todo", "in_progress", "done"} {
		value := status
		if _, err := s.UpdateTask(context.Background(), task.ID, domain.UpdateTaskInput{Status: &value}); err != nil {
			t.Fatalf("status %s: %v", status, err)
		}
	}
	bad := "blocked"
	if _, err := s.UpdateTask(context.Background(), task.ID, domain.UpdateTaskInput{Status: &bad}); err != ErrInvalidInput {
		t.Fatalf("error = %v, want ErrInvalidInput", err)
	}
}

func TestMissingProjectAndTask(t *testing.T) {
	s := newTestService()
	if _, err := s.CreateTask(context.Background(), "missing", domain.CreateTaskInput{Title: "T"}); err != ErrProjectNotFound {
		t.Fatalf("error = %v", err)
	}
	if _, err := s.GetTask(context.Background(), "missing"); err != ErrTaskNotFound {
		t.Fatalf("error = %v", err)
	}
}
