package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"runtime/debug"
	"sync/atomic"
	"time"
)

type requestIDKey struct{}

var (
	reqIDPrefix  string
	reqIDCounter atomic.Uint64
)

func init() {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err == nil {
		reqIDPrefix = hex.EncodeToString(b)
	} else {
		reqIDPrefix = "0123456789abcdef"
	}
}

// Chain middleware ordering from outer to inner:
// Recovery -> RequestID -> RequestLog -> Metrics -> RequestTimeout -> next
func Chain(next http.Handler, logger *slog.Logger, timeout time.Duration) http.Handler {
	h := next
	h = RequestTimeout(timeout)(h)
	h = Metrics(h)
	h = RequestLog(logger)(h)
	h = RequestID(h)
	h = Recovery(logger)(h)
	return h
}

func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			id = newRequestID()
		}
		// Store ID in request context for downstream handlers
		ctx := context.WithValue(r.Context(), requestIDKey{}, id)
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func RequestIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey{}).(string); ok {
		return id
	}
	return ""
}

// ID provides backward compatibility for legacy callers.
func ID(ctx context.Context) string { return RequestIDFromContext(ctx) }

func RequestTimeout(timeout time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.TimeoutHandler(next, timeout, `{"error":{"code":"timeout","message":"request timed out"}}`)
	}
}

func Recovery(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					logger.ErrorContext(r.Context(), "panic recovered",
						"panic", rec,
						"stack", string(debug.Stack()))
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusInternalServerError)
					_, _ = w.Write([]byte(`{"error":{"code":"internal_error","message":"internal server error"}}`))
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// responseRecorder captures HTTP status code for RequestLog.
type responseRecorder struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (r *responseRecorder) WriteHeader(code int) {
	if !r.wrote {
		r.status = code
		r.wrote = true
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	if !r.wrote {
		r.status = http.StatusOK
		r.wrote = true
	}
	return r.ResponseWriter.Write(b)
}

func RequestLog(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &responseRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r)

			attrs := []any{
				"method", r.Method,
				"path", r.URL.Path,
				"status", rec.status,
				"duration_ms", time.Since(start).Milliseconds(),
				"remote_addr", r.RemoteAddr,
			}
			if rec.status >= 500 {
				logger.ErrorContext(r.Context(), "http request", attrs...)
			} else if rec.status >= 400 {
				logger.WarnContext(r.Context(), "http request", attrs...)
			} else {
				logger.InfoContext(r.Context(), "http request", attrs...)
			}
		})
	}
}

// newRequestID generates high-throughput, low-allocation request IDs via atomic counters.
func newRequestID() string {
	seq := reqIDCounter.Add(1)
	var buf [32]byte
	copy(buf[:16], reqIDPrefix)
	const hextable = "0123456789abcdef"
	for i := 31; i >= 16; i-- {
		buf[i] = hextable[seq&0x0f]
		seq >>= 4
	}
	return string(buf[:])
}

// NewRequestID exports request ID generation for benchmarks and testing.
func NewRequestID() string {
	return newRequestID()
}

