package cache

import (
	"context"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"workhub-service/internal/domain"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// TestProjectSummaryCacheHitMissTTLAndInvalidation tests project summary cache lifecycle:
// hits, misses, TTL expiry, cache invalidation, and fail-open behavior during Redis outage.
func TestProjectSummaryCacheHitMissTTLAndInvalidation(t *testing.T) {
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	cache := NewProjectSummaryCache(client, 10*time.Second, slog.Default(), nil)
	ctx := context.Background()
	summary := domain.ProjectSummary{ProjectID: "42", TaskCount: 4, CompletedCount: 1, OpenCount: 3}

	if _, ok := cache.Get(ctx, "42"); ok {
		t.Fatal("empty cache returned a hit")
	}
	cache.Set(ctx, summary)
	got, ok := cache.Get(ctx, "42")
	if !ok || got != summary {
		t.Fatalf("cache hit = %+v, %v", got, ok)
	}
	cache.Invalidate(ctx, "42")
	if _, ok := cache.Get(ctx, "42"); ok {
		t.Fatal("invalidated key returned a hit")
	}
	cache.Set(ctx, summary)
	mini.FastForward(11 * time.Second)
	if _, ok := cache.Get(ctx, "42"); ok {
		t.Fatal("expired key returned a hit")
	}

	mini.Close() // Shut down Redis to simulate backend outage

	if _, ok := cache.Get(ctx, "42"); ok {
		t.Fatal("offline Redis returned a hit")
	}
	// Set and Invalidate intentionally fail closed: the database remains the source of truth.
	cache.Set(ctx, summary)
	cache.Invalidate(ctx, "42")
}

// TestCacheStampedeProtection tests thundering herd prevention, ensuring compute runs once across concurrent callers.
func TestCacheStampedeProtection(t *testing.T) {
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	cache := NewProjectSummaryCache(client, 10*time.Second, slog.Default(), nil)
	ctx := context.Background()

	var computeCalls int32
	projectID := "proj-stampede-1"

	// Simulate 50 concurrent goroutines requesting the same project summary upon cache miss
	const concurrentRequests = 50
	errCh := make(chan error, concurrentRequests)
	doneCh := make(chan domain.ProjectSummary, concurrentRequests)

	for i := 0; i < concurrentRequests; i++ {
		go func() {
			summary, err := cache.GetOrCompute(ctx, projectID, func(ctx context.Context) (domain.ProjectSummary, error) {
				// Increment counter to verify actual execution count
				atomic.AddInt32(&computeCalls, 1)
				time.Sleep(20 * time.Millisecond) // Simulate DB latency
				return domain.ProjectSummary{ProjectID: projectID, TaskCount: 100}, nil
			})
			if err != nil {
				errCh <- err
				return
			}
			doneCh <- summary
		}()
	}

	for i := 0; i < concurrentRequests; i++ {
		select {
		case err := <-errCh:
			t.Fatalf("unexpected error: %v", err)
		case summary := <-doneCh:
			if summary.TaskCount != 100 {
				t.Fatalf("expected TaskCount 100, got %d", summary.TaskCount)
			}
		}
	}

	// Verification: across 50 concurrent requests, computeFn must execute exactly once
	if calls := atomic.LoadInt32(&computeCalls); calls != 1 {
		t.Fatalf("Cache Stampede protection failed! Expected 1 compute call, got %d", calls)
	}
}
