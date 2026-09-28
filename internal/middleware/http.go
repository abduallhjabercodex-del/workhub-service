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

// Chain — الترتيب من الخارج إلى الداخل:
// Recovery → RequestID → RequestLog → Metrics → RequestTimeout → next
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
		// نضع الـ ID في context فقط — `requestID(r)` يجب أن يقرأ من context.
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

// ID — للتوافق مع الكود القديم.
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

// responseRecorder — يلتقط status code لـ RequestLog.
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

// newRequestID — مولد عالي الأداء منخفض التخصيص خالٍ من أقفال النواة (Zero-Lock Atomic Generator)
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

// NewRequestID — دالة عامة تتيح اختبار واستخدام المولد المحسن
func NewRequestID() string {
	return newRequestID()
}

/*
================================================================================
 النسخة المقابلة باستخدام إطار عمل Fiber (للمقارنة فقط - معلّقة ومظللة)
 FIBER EQUIVALENT IMPLEMENTATION (FOR COMPARISON ONLY - COMMENTED OUT)
================================================================================

import (
	"log/slog"
	"runtime/debug"
	"time"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/requestid"
)

// RequestIDFiber handles request ID generation and header injection in Fiber
func RequestIDFiber() fiber.Handler {
	return requestid.New(requestid.Config{
		Header:     "X-Request-ID",
		Generator:  newRequestID,
		ContextKey: "requestid",
	})
}

func RequestIDFromFiber(c *fiber.Ctx) string {
	if id, ok := c.Locals("requestid").(string); ok && id != "" {
		return id
	}
	return c.Get("X-Request-ID")
}

// RecoveryFiber handles panic recovery in Fiber
func RecoveryFiber(logger *slog.Logger) fiber.Handler {
	return func(c *fiber.Ctx) (err error) {
		defer func() {
			if rec := recover(); rec != nil {
				logger.ErrorContext(c.UserContext(), "panic recovered",
					"panic", rec,
					"stack", string(debug.Stack()))

				err = c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
					"error": fiber.Map{
						"code":    "internal_error",
						"message": "internal server error",
					},
				})
			}
		}()
		return c.Next()
	}
}

// RequestLogFiber logs incoming HTTP requests with latency and status code
func RequestLogFiber(logger *slog.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()
		err := c.Next()

		status := c.Response().StatusCode()
		attrs := []any{
			"method", c.Method(),
			"path", c.Path(),
			"status", status,
			"duration_ms", time.Since(start).Milliseconds(),
			"remote_addr", c.IP(),
		}

		if status >= 500 {
			logger.ErrorContext(c.UserContext(), "http request", attrs...)
		} else if status >= 400 {
			logger.WarnContext(c.UserContext(), "http request", attrs...)
		} else {
			logger.InfoContext(c.UserContext(), "http request", attrs...)
		}

		return err
	}
}
*/
