package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"workhub-service/internal/domain"
)

// BenchmarkListTasks يختبر قياس أداء استرجاع وتصفية المهام (Benchmark) من مستودع الذاكرة (In-Memory Repository) عند وجود عدد كبير من المهام مع تصفية وتقسيم الصفحات.
func BenchmarkListTasks(b *testing.B) {
	repo := NewMemoryRepository()
	for i := 0; i < 1000; i++ {
		repo.CreateTask(context.Background(), domain.Task{ID: fmt.Sprintf("task-%d", i), ProjectID: "project-1", Title: "benchmark task", Status: "todo", CreatedAt: time.Now()})
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = repo.ListTasks(context.Background(), "project-1", "todo", "benchmark", "title", 1, 20)
	}
}
