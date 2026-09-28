package performance

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"workhub-service/internal/handler"
	"workhub-service/internal/middleware"
)

// legacyNewRequestID — تحاكي دالة توليد الـ Request ID القديمة المعتمدة على crypto/rand
func legacyNewRequestID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "unknown"
	}
	return hex.EncodeToString(b)
}

// 1. قياس دالة توليد المعرف القديمة
func BenchmarkRequestID_Legacy(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = legacyNewRequestID()
	}
}

// 1.ب قياس دالة توليد المعرف المحسنة بعد حل عنق الزجاجة
func BenchmarkRequestID_Optimized(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = middleware.NewRequestID()
	}
}

// 2. قياس سلسلة Middleware كاملة
func BenchmarkMiddlewareChain(b *testing.B) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	dummyHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	h := middleware.Chain(dummyHandler, logger, 5*time.Second)
	req := httptest.NewRequest("GET", "/api/v1/test", nil)
	rec := httptest.NewRecorder()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rec.Body.Reset()
		h.ServeHTTP(rec, req)
	}
}

// 3. قياس نقطة فحص الصحة Health
func BenchmarkHealthEndpoint(b *testing.B) {
	req := httptest.NewRequest("GET", "/health", nil)
	rec := httptest.NewRecorder()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rec.Body.Reset()
		handler.Health(rec, req)
	}
}

// 4. قياس استخراج Request ID من السياق
func BenchmarkRequestIDFromContext(b *testing.B) {
	ctx := context.Background()
	req := httptest.NewRequest("GET", "/test", nil).WithContext(ctx)
	handler := middleware.RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = middleware.RequestIDFromContext(r.Context())
	}))
	rec := httptest.NewRecorder()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		handler.ServeHTTP(rec, req)
	}
}
