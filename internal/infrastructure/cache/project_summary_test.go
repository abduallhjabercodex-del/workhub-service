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

// TestProjectSummaryCacheHitMissTTLAndInvalidation يختبر دورة حياة الذاكرة المؤقتة (الكاش) لملخص المشروع: حالات الإصابة (Hit)، الفقدان (Miss)، انقضاء الصلاحية (TTL)، الإبطال (Invalidation)، وسلوك الكاش عند انقطاع خادم Redis.
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

	mini.Close() // mini.Close(): نقوم بسحق وإغلاق خادم Redis تماماً لمحاكاة عطل كوارثي بانهيار Redis في الإنتاج!

	if _, ok := cache.Get(ctx, "42"); ok {
		t.Fatal("offline Redis returned a hit")
	}
	// Set and Invalidate intentionally fail closed: the database remains the source of truth.
	cache.Set(ctx, summary)
	cache.Invalidate(ctx, "42")
}

// TestCacheStampedeProtection يختبر حماية الكاش من ظاهرة تزاحم الطلبات المتزامنة (Cache Stampede / Thundering Herd) والتأكد من إنجاز الاستعلام مرة واحدة فقط في دالة التجميد عبر قفل المزاحمة.
func TestCacheStampedeProtection(t *testing.T) {
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	cache := NewProjectSummaryCache(client, 10*time.Second, slog.Default(), nil)
	ctx := context.Background()

	var computeCalls int32
	projectID := "proj-stampede-1"

	// محاكاة 50 goroutine تطلب ملخص نفس المشروع متزامنة عند حدث Cache Miss
	const concurrentRequests = 50
	errCh := make(chan error, concurrentRequests)
	doneCh := make(chan domain.ProjectSummary, concurrentRequests)

	for i := 0; i < concurrentRequests; i++ {
		go func() {
			summary, err := cache.GetOrCompute(ctx, projectID, func(ctx context.Context) (domain.ProjectSummary, error) {
				// زيادة العداد لتتبع عدد المرات التي تم تنفيذ الاستعلام فيها فعلياً
				atomic.AddInt32(&computeCalls, 1)
				time.Sleep(20 * time.Millisecond) // محاكاة زمن استعلام قاعدة البيانات
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

	// الإثبات العملي: بالرغم من وجود 50 طلب متزامن، دالة computeFn يجب أن تُستدعى مَرّة واحِدَة فَقَط!
	if calls := atomic.LoadInt32(&computeCalls); calls != 1 {
		t.Fatalf("Cache Stampede protection failed! Expected 1 compute call, got %d", calls)
	}
}
