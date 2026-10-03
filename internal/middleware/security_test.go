package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestSecurityHeaders verifies standard OWASP security response headers are attached.
func TestSecurityHeaders(t *testing.T) {
	middleware := SecurityHeaders("default-src 'self'")
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	headers := map[string]string{
		"X-Frame-Options":         "DENY",
		"X-Content-Type-Options":  "nosniff",
		"X-XSS-Protection":        "0",
		"Content-Security-Policy": "default-src 'self'",
		"Referrer-Policy":         "strict-origin-when-cross-origin",
	}

	for k, expected := range headers {
		if got := rec.Header().Get(k); got != expected {
			t.Errorf("header %s = %q, want %q", k, got, expected)
		}
	}
}

// TestRateLimiter verifies token bucket limits requests per client key and returns 429 when capacity is exceeded.
func TestRateLimiter(t *testing.T) {
	limiter := NewRateLimiter(3, 10*time.Minute)
	middleware := RateLimit(limiter, nil)

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "192.168.1.1:12345"

	// First 3 requests should pass
	for i := 0; i < 3; i++ {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d status = %d, want 200", i+1, rec.Code)
		}
	}

	// 4th request should fail with 429
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("request 4 status = %d, want 429", rec.Code)
	}
}
