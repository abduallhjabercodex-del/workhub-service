package middleware

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestRequestIDIsGeneratedAndReturned tests generation and response header propagation of X-Request-ID.
func TestRequestIDIsGeneratedAndReturned(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	res := httptest.NewRecorder()
	RequestID(next).ServeHTTP(res, req)
	if res.Header().Get("X-Request-ID") == "" {
		t.Fatal("request id was not returned")
	}
}

// TestRecoveryReturnsServerError tests graceful panic recovery returning 500 without crashing server.
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

// TestTimeoutMiddleware tests cancellation of requests exceeding configured timeout returning 503.
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
