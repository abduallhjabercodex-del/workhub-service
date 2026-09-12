package cache

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"workhub-service/internal/domain"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestProjectSummaryCacheHitMissTTLAndInvalidation(t *testing.T) {
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	cache := NewProjectSummaryCache(client, 10*time.Second, slog.Default())
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

	mini.Close() // mini.Close(): نقوم بسحق وإغلاق خادم Redis تماماً لمحاكاة عطل كوارثي بانهيار Redis في الإنتاج!

	if _, ok := cache.Get(ctx, "42"); ok {
		t.Fatal("offline Redis returned a hit")
	}
	// Set and Invalidate intentionally fail closed: the database remains the source of truth.
	cache.Set(ctx, summary)
	cache.Invalidate(ctx, "42")
}
