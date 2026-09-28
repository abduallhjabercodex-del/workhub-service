package middleware

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"workhub-service/internal/infrastructure/cache"
	"workhub-service/internal/infrastructure/metrics"
)

// maxIdempotencyBodySize — الحد الأقصى لحجم الـ body لقراءة الـ fingerprint.
const maxIdempotencyBodySize = 1 << 20 // 1 MB

// idempotencyResponseRecorder — يلتقط status + body مع تمرير الكتابة للـ writer الحقيقي.
type idempotencyResponseRecorder struct {
	http.ResponseWriter
	statusCode  int
	body        *bytes.Buffer
	wroteHeader bool
}

func newIdempotencyResponseRecorder(w http.ResponseWriter) *idempotencyResponseRecorder {
	return &idempotencyResponseRecorder{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
		body:           bytes.NewBuffer(nil),
	}
}

func (rw *idempotencyResponseRecorder) WriteHeader(code int) {
	if rw.wroteHeader {
		return
	}
	rw.statusCode = code
	rw.wroteHeader = true
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *idempotencyResponseRecorder) Write(b []byte) (int, error) {
	if !rw.wroteHeader {
		rw.wroteHeader = true
	}
	rw.body.Write(b)
	return rw.ResponseWriter.Write(b)
}

// Flush — يمرّر Flush للـ writer الحقيقي إن دعمه (streaming/SSE).
func (rw *idempotencyResponseRecorder) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Idempotency — يضمن تنفيذ POST/PUT/PATCH مرة واحدة لكل (user, idempotency-key, body).
//
// النمط:
//  1. Fingerprint body.
//  2. Get: cached response? → replay.
//  3. Begin: SET NX atomic reserve.
//     - acquired=true  → execute.
//     - ErrInProgress  → 409 Conflict.
//     - ErrFingerprintMismatch → 409 Conflict.
//  4. Execute handler.
//  5. Save (2xx only) أو Fail (غيره).
//
// Fail-Open: إذا فشل Redis في Get أو Begin، نكمل بدون idempotency مع تحذير.
func Idempotency(store cache.IdempotencyStore, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 0) شروط مسبقة.
			if store == nil {
				next.ServeHTTP(w, r)
				return
			}
			if r.Method != http.MethodPost &&
				r.Method != http.MethodPut &&
				r.Method != http.MethodPatch {
				next.ServeHTTP(w, r)
				return
			}

			key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
			if key == "" {
				next.ServeHTTP(w, r)
				return
			}

			user, ok := UserFromContext(r.Context())
			if !ok || user.ID == "" {
				logger.WarnContext(r.Context(), "idempotency key provided without authenticated user")
				next.ServeHTTP(w, r)
				return
			}

			// 1) اقرأ الـ body لـ fingerprint (مع حد أقصى).
			body, err := io.ReadAll(io.LimitReader(r.Body, maxIdempotencyBodySize+1))
			if err != nil {
				logger.WarnContext(r.Context(), "idempotency: read body failed", "error", err)
				writeIdempotencyError(w, http.StatusBadRequest, "invalid_input", "failed to read request body")
				return
			}
			if len(body) > maxIdempotencyBodySize {
				logger.WarnContext(r.Context(), "idempotency: body too large", "size", len(body))
				writeIdempotencyError(w, http.StatusRequestEntityTooLarge, "payload_too_large", "request body too large for idempotency")
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(body))
			fingerprint := cache.Fingerprint(body)

			ctx := r.Context()

			// 2) اقرأ النتيجة المخزّنة.
			cached, err := store.Get(ctx, user.ID, key, fingerprint)
			if err != nil {
				if errors.Is(err, cache.ErrFingerprintMismatch) {
					metrics.IdempotencyRequestsTotal.WithLabelValues("mismatch").Inc()
					writeIdempotencyError(w, http.StatusConflict,
						"idempotency_key_mismatch",
						"idempotency key was reused with a different request body")
					return
				}
				// Redis error → fail-open.
				logger.WarnContext(ctx, "idempotency: get failed (fail-open)", "error", err, "key", key)
				next.ServeHTTP(w, r)
				return
			}
			if cached != nil {
				metrics.IdempotencyRequestsTotal.WithLabelValues("replayed").Inc()
				replayResponse(w, cached)
				logger.InfoContext(ctx, "idempotent replay", "key", key)
				return
			}

			// 3) حاول حجز المفتاح ذرّيًا.
			acquired, err := store.Begin(ctx, user.ID, key, fingerprint)
			if err != nil {
				switch {
				case errors.Is(err, cache.ErrInProgress):
					metrics.IdempotencyRequestsTotal.WithLabelValues("in_progress").Inc()
					w.Header().Set("Retry-After", "1")
					writeIdempotencyError(w, http.StatusConflict,
						"idempotency_in_progress",
						"request with this idempotency key is already in progress")
					return
				case errors.Is(err, cache.ErrFingerprintMismatch):
					metrics.IdempotencyRequestsTotal.WithLabelValues("mismatch").Inc()
					writeIdempotencyError(w, http.StatusConflict,
						"idempotency_key_mismatch",
						"idempotency key was reused with a different request body")
					return
				default:
					logger.WarnContext(ctx, "idempotency: begin failed (fail-open)", "error", err, "key", key)
					next.ServeHTTP(w, r)
					return
				}
			}
			if !acquired {
				// سباق نادر: ظهرت النتيجة بين Get و Begin. أعد القراءة.
				cached, _ := store.Get(ctx, user.ID, key, fingerprint)
				if cached != nil {
					metrics.IdempotencyRequestsTotal.WithLabelValues("replayed").Inc()
					replayResponse(w, cached)
					return
				}
				next.ServeHTTP(w, r)
				return
			}

			// 4) حجزنا المفتاح — نضمن تحريره إذا خرجنا بشكل غير طبيعي (panic، إلخ).
			committed := false
			defer func() {
				if !committed {
					// WithoutCancel: احفظ التحرير حتى لو أُلغي ctx.
					_ = store.Fail(context.WithoutCancel(ctx), user.ID, key)
				}
			}()

			rec := newIdempotencyResponseRecorder(w)
			next.ServeHTTP(rec, r)

			// 5) احفظ 2xx فقط. 4xx/5xx → حرّر الحجز للسماح بإعادة المحاولة.
			if rec.statusCode >= 200 && rec.statusCode < 300 {
				saveErr := store.Save(ctx, user.ID, key, &cache.IdempotencyResponse{
					StatusCode:  rec.statusCode,
					Headers:     cloneHeaders(rec.Header()),
					Body:        rec.body.Bytes(),
					Fingerprint: fingerprint,
				})
				if saveErr != nil {
					logger.ErrorContext(ctx, "idempotency: save failed", "error", saveErr, "key", key)
					_ = store.Fail(ctx, user.ID, key)
				}
				committed = true
				return
			}

			// 4xx/5xx → لا نخزّن (client يجب أن يصحّح ويستخدم مفتاحًا جديدًا).
			metrics.IdempotencyRequestsTotal.WithLabelValues("failed").Inc()
			_ = store.Fail(ctx, user.ID, key)
			committed = true
		})
	}
}

// replayResponse — يعيد كتابة استجابة مخزّنة.
func replayResponse(w http.ResponseWriter, resp *cache.IdempotencyResponse) {
	w.Header().Set("Idempotent-Replay", "true")
	for k, vs := range resp.Headers {
		// Content-Length سيُضبط تلقائيًا من المُغلّف.
		if strings.EqualFold(k, "Content-Length") {
			continue
		}
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	if len(resp.Body) > 0 {
		_, _ = w.Write(resp.Body)
	}
}

// cloneHeaders — نسخة مستقلة من الـ headers.
func cloneHeaders(h http.Header) map[string][]string {
	out := make(map[string][]string, len(h))
	for k, vs := range h {
		cp := make([]string, len(vs))
		copy(cp, vs)
		out[k] = cp
	}
	return out
}

// writeIdempotencyError — استجابة JSON متسقة.
func writeIdempotencyError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`{"error":{"code":"` + code + `","message":"` + message + `"}}`))
}

/*
================================================================================
 النسخة المقابلة باستخدام إطار عمل Fiber (للمقارنة فقط - معلّقة ومظللة)
 FIBER EQUIVALENT IMPLEMENTATION (FOR COMPARISON ONLY - COMMENTED OUT)
================================================================================

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"github.com/gofiber/fiber/v2"
	"workhub-service/internal/infrastructure/cache"
	"workhub-service/internal/infrastructure/metrics"
)

// IdempotencyFiber guarantees idempotent execution of mutating endpoints in Fiber.
// Note: In Fiber, c.Body() and c.Response().Body() are available directly, eliminating
// the need for custom response recorders and body wrappers.
func IdempotencyFiber(store cache.IdempotencyStore, logger *slog.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if store == nil {
			return c.Next()
		}

		method := c.Method()
		if method != fiber.MethodPost && method != fiber.MethodPut && method != fiber.MethodPatch {
			return c.Next()
		}

		key := strings.TrimSpace(c.Get("Idempotency-Key"))
		if key == "" {
			return c.Next()
		}

		user, ok := UserFromFiberContext(c)
		if !ok || user.ID == "" {
			logger.WarnContext(c.UserContext(), "idempotency key provided without authenticated user")
			return c.Next()
		}

		body := c.Body()
		if len(body) > maxIdempotencyBodySize {
			logger.WarnContext(c.UserContext(), "idempotency: body too large", "size", len(body))
			return c.Status(fiber.StatusRequestEntityTooLarge).JSON(fiber.Map{
				"error": fiber.Map{
					"code":    "payload_too_large",
					"message": "request body too large for idempotency",
				},
			})
		}
		fingerprint := cache.Fingerprint(body)
		ctx := c.UserContext()

		// Check cached response
		cached, err := store.Get(ctx, user.ID, key, fingerprint)
		if err != nil {
			if errors.Is(err, cache.ErrFingerprintMismatch) {
				metrics.IdempotencyRequestsTotal.WithLabelValues("mismatch").Inc()
				return c.Status(fiber.StatusConflict).JSON(fiber.Map{
					"error": fiber.Map{
						"code":    "idempotency_key_mismatch",
						"message": "idempotency key was reused with a different request body",
					},
				})
			}
			logger.WarnContext(ctx, "idempotency: get failed (fail-open)", "error", err, "key", key)
			return c.Next()
		}
		if cached != nil {
			metrics.IdempotencyRequestsTotal.WithLabelValues("replayed").Inc()
			c.Set("Idempotent-Replay", "true")
			for k, vs := range cached.Headers {
				if !strings.EqualFold(k, "Content-Length") && len(vs) > 0 {
					c.Set(k, vs[0])
				}
			}
			return c.Status(cached.StatusCode).Send(cached.Body)
		}

		// Reserve atomic key
		acquired, err := store.Begin(ctx, user.ID, key, fingerprint)
		if err != nil {
			switch {
			case errors.Is(err, cache.ErrInProgress):
				metrics.IdempotencyRequestsTotal.WithLabelValues("in_progress").Inc()
				c.Set("Retry-After", "1")
				return c.Status(fiber.StatusConflict).JSON(fiber.Map{
					"error": fiber.Map{
						"code":    "idempotency_in_progress",
						"message": "request with this idempotency key is already in progress",
					},
				})
			case errors.Is(err, cache.ErrFingerprintMismatch):
				metrics.IdempotencyRequestsTotal.WithLabelValues("mismatch").Inc()
				return c.Status(fiber.StatusConflict).JSON(fiber.Map{
					"error": fiber.Map{
						"code":    "idempotency_key_mismatch",
						"message": "idempotency key was reused with a different request body",
					},
				})
			default:
				logger.WarnContext(ctx, "idempotency: begin failed (fail-open)", "error", err, "key", key)
				return c.Next()
			}
		}
		if !acquired {
			cached, _ := store.Get(ctx, user.ID, key, fingerprint)
			if cached != nil {
				metrics.IdempotencyRequestsTotal.WithLabelValues("replayed").Inc()
				c.Set("Idempotent-Replay", "true")
				return c.Status(cached.StatusCode).Send(cached.Body)
			}
			return c.Next()
		}

		committed := false
		defer func() {
			if !committed {
				_ = store.Fail(context.WithoutCancel(ctx), user.ID, key)
			}
		}()

		// Execute downstream handlers
		execErr := c.Next()
		statusCode := c.Response().StatusCode()

		if statusCode >= 200 && statusCode < 300 {
			headers := make(map[string][]string)
			c.Response().Header.VisitAll(func(k, v []byte) {
				headers[string(k)] = []string{string(v)}
			})

			_ = store.Save(ctx, user.ID, key, &cache.IdempotencyResponse{
				StatusCode:  statusCode,
				Headers:     headers,
				Body:        c.Response().Body(),
				Fingerprint: fingerprint,
			})
			committed = true
			return execErr
		}

		metrics.IdempotencyRequestsTotal.WithLabelValues("failed").Inc()
		_ = store.Fail(ctx, user.ID, key)
		committed = true
		return execErr
	}
}
*/
