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

// maxIdempotencyBodySize is the maximum request body size read for payload fingerprinting.
const maxIdempotencyBodySize = 1 << 20 // 1 MB

// idempotencyResponseRecorder captures status and response body while streaming to downstream writer.
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

// Flush forwards flush signal to underlying ResponseWriter if supported (streaming/SSE).
func (rw *idempotencyResponseRecorder) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Idempotency ensures POST/PUT/PATCH execution runs exactly once per (user, idempotency-key, body).
//
// Lifecycle:
//  1. Fingerprint request body.
//  2. Get: check for cached response -> replay immediately if found.
//  3. Begin: SET NX atomic reserve.
//     - acquired=true  -> proceed with handler execution.
//     - ErrInProgress  -> 409 Conflict.
//     - ErrFingerprintMismatch -> 409 Conflict.
//  4. Execute downstream handler.
//  5. Save (2xx only) or Fail (non-2xx).
//
// Fail-Open: if Redis errors during Get or Begin, request proceeds without idempotency with warning.
func Idempotency(store cache.IdempotencyStore, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 0) Preconditions
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

			// 1) Read request body for fingerprinting (with bounded limit)
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

			// 2) Check for cached response
			cached, err := store.Get(ctx, user.ID, key, fingerprint)
			if err != nil {
				if errors.Is(err, cache.ErrFingerprintMismatch) {
					metrics.IdempotencyRequestsTotal.WithLabelValues("mismatch").Inc()
					writeIdempotencyError(w, http.StatusConflict,
						"idempotency_key_mismatch",
						"idempotency key was reused with a different request body")
					return
				}
				// Redis error -> fail-open
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

			// 3) Attempt atomic key acquisition
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
				// Rare race condition: result completed between Get and Begin; re-read
				cached, _ := store.Get(ctx, user.ID, key, fingerprint)
				if cached != nil {
					metrics.IdempotencyRequestsTotal.WithLabelValues("replayed").Inc()
					replayResponse(w, cached)
					return
				}
				next.ServeHTTP(w, r)
				return
			}

			// 4) Key acquired: ensure cleanup on abnormal exit (panic, timeout)
			committed := false
			defer func() {
				if !committed {
					// WithoutCancel: ensure unlock even if request ctx was cancelled
					_ = store.Fail(context.WithoutCancel(ctx), user.ID, key)
				}
			}()

			rec := newIdempotencyResponseRecorder(w)
			next.ServeHTTP(rec, r)

			// 5) Cache 2xx only; 4xx/5xx release key to allow retries
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

			// 4xx/5xx: do not cache (client should correct request and retry with fresh key)
			metrics.IdempotencyRequestsTotal.WithLabelValues("failed").Inc()
			_ = store.Fail(ctx, user.ID, key)
			committed = true
		})
	}
}

// replayResponse writes cached response back to client.
func replayResponse(w http.ResponseWriter, resp *cache.IdempotencyResponse) {
	w.Header().Set("Idempotent-Replay", "true")
	for k, vs := range resp.Headers {
		// Content-Length is managed automatically by writer
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

// cloneHeaders returns an independent copy of headers.
func cloneHeaders(h http.Header) map[string][]string {
	out := make(map[string][]string, len(h))
	for k, vs := range h {
		cp := make([]string, len(vs))
		copy(cp, vs)
		out[k] = cp
	}
	return out
}

// writeIdempotencyError writes a consistent JSON error response.
func writeIdempotencyError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`{"error":{"code":"` + code + `","message":"` + message + `"}}`))
}

