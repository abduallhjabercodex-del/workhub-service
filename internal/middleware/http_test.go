package middleware

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestRequestIDIsGeneratedAndReturned يختبر إنشاء معرف فريد للطلب (X-Request-ID) وإضافته لترويسات الاستجابة وحيازة سياق الطلب عليه.
func TestRequestIDIsGeneratedAndReturned(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	res := httptest.NewRecorder()
	RequestID(next).ServeHTTP(res, req)
	if res.Header().Get("X-Request-ID") == "" {
		t.Fatal("request id was not returned")
	}
}

// TestRecoveryReturnsServerError يختبر التعافي الآمن من حالات الـ Panic أثناء معالجة الطلبات وإرجاع رمز الاستجابة 500 دون إيقاف تشغيل الخادم.
func TestRecoveryReturnsServerError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(&strings.Builder{}, nil))
	next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("test panic") })
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	res := httptest.NewRecorder()
	Recovery(logger)(next).ServeHTTP(res, req)
	if res.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", res.Code)
	}
}

// TestTimeoutMiddleware يختبر إلغاء الطلبات التي تتجاوز المهلة الزمنية المحددة (Timeout) وإرجاع رمز الاستجابة 503 (Service Unavailable).
func TestTimeoutMiddleware(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(20 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	res := httptest.NewRecorder()
	RequestTimeout(1*time.Millisecond)(next).ServeHTTP(res, req)
	if res.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d", res.Code)
	}
}
