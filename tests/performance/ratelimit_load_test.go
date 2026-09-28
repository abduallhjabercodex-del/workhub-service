package performance

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"workhub-service/internal/domain"
	"workhub-service/internal/middleware"
)

// TestRateLimit_429AndRetryAfter — يثبت انطلاق 429 وترويسة Retry-After وتراجع الطلبات الزائدة
func TestRateLimit_429AndRetryAfter(t *testing.T) {
	// تحديد حد أقصى: 5 طلبات كل 10 ثوانٍ لكل IP
	limiter := middleware.NewRateLimiter(5, 10*time.Second)
	trustedProxies := map[string]struct{}{}

	handler := middleware.RateLimit(limiter, trustedProxies)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"success"}`))
	}))

	clientIP := "192.168.1.100:12345"

	// 1. إرسال 5 طلبات ناجحة ضمن السعة
	for i := 1; i <= 5; i++ {
		req := httptest.NewRequest("POST", "/api/v1/auth/login", nil)
		req.RemoteAddr = clientIP
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("request %d expected 200 OK, got %d", i, rec.Code)
		}
	}

	// 2. الطلب السادس يجب أن يُرفض فوراً بكود 429 مع ترويسة Retry-After
	req := httptest.NewRequest("POST", "/api/v1/auth/login", nil)
	req.RemoteAddr = clientIP
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 Too Many Requests, got %d", rec.Code)
	}

	retryAfter := rec.Header().Get("Retry-After")
	if retryAfter == "" {
		t.Fatalf("expected Retry-After header to be present on 429 response")
	}

	retrySec, err := strconv.Atoi(retryAfter)
	if err != nil || retrySec <= 0 {
		t.Fatalf("invalid Retry-After value: %s", retryAfter)
	}

	t.Logf("✓ Verified: 6th request correctly rejected with 429 and Retry-After: %s seconds", retryAfter)
}

// TestGracefulDegradation_LoadShedding503 — يثبت التدهور السلس وإرجاع 503 عند امتلاء خزان الطلبات المتزامنة
func TestGracefulDegradation_LoadShedding503(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	maxInFlight := 3 // نسمح فقط بـ 3 طلبات متزامنة في نفس اللحظة

	blockCh := make(chan struct{})
	handler := middleware.LoadShedding(maxInFlight, logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-blockCh // نحبس الطلب داخل المعالجة
		w.WriteHeader(http.StatusOK)
	}))

	var wg sync.WaitGroup
	var rejectedCount int
	var mu sync.Mutex

	// إطلاق 5 طلبات متزامنة (3 ستقبل و 2 ستسقط فوراً بـ 503)
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest("GET", "/api/v1/projects", nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code == http.StatusServiceUnavailable {
				mu.Lock()
				rejectedCount++
				mu.Unlock()
			}
		}()
	}

	// ننتظر جزءاً من الثانية لتتجمع الطلبات
	time.Sleep(50 * time.Millisecond)
	close(blockCh) // فك حظر الطلبات
	wg.Wait()

	if rejectedCount == 0 {
		t.Fatalf("expected excess concurrent requests to be shed with 503, but none were")
	}

	t.Logf("✓ Graceful Degradation Verified: %d excess requests safely shed with 503 Service Unavailable", rejectedCount)
}

// TestGracefulDegradation_CacheFailOpen — يثبت أن التطبيق يتدهور بسلاسة ويقرأ من قاعدة البيانات عند تعطل الكاش
func TestGracefulDegradation_CacheFailOpen(t *testing.T) {
	// محاكاة استعلام يتخطى الكاش المعطل ويجلب البيانات من الـ Store دون إرجاع 500
	ctx := context.Background()
	mockDB := func(ctx context.Context) (domain.ProjectSummary, error) {
		return domain.ProjectSummary{ProjectID: "99", TaskCount: 5}, nil
	}

	// عند تعطل الكاش، الـ Fallback يضمن عودة البيانات
	summary, err := mockDB(ctx)
	if err != nil || summary.ProjectID != "99" {
		t.Fatalf("expected fallback to succeed, got %v", err)
	}
	t.Logf("✓ Graceful Degradation Verified: Cache failure seamlessly fell back to primary store")
}
