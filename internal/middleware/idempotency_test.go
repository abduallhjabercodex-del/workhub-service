package middleware

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"workhub-service/internal/infrastructure/cache"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestIdempotency(t *testing.T) {
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	store := cache.NewRedisIdempotencyStore(client, time.Minute)

	logger := slog.Default()
	middleware := Idempotency(store, logger)

	handlerCalled := 0
	targetHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled++
		w.Header().Set("X-Custom", "test")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte("success"))
	})

	handler := middleware(targetHandler)

	doRequest := func(method, key string, userID string) (*http.Response, string) {
		req := httptest.NewRequest(method, "http://example.com/api", nil)
		if key != "" {
			req.Header.Set("Idempotency-Key", key)
		}
		if userID != "" {
			ctx := context.WithValue(req.Context(), userCtxKey{}, AuthenticatedUser{ID: userID})
			req = req.WithContext(ctx)
		}

		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		resp := w.Result()
		body, _ := io.ReadAll(resp.Body)
		return resp, string(body)
	}

	// 1. Missing Key -> runs handler normally
	resp, body := doRequest(http.MethodPost, "", "user1")
	if resp.StatusCode != http.StatusCreated || body != "success" || handlerCalled != 1 {
		t.Fatalf("expected handler to run normally, got %d, %s, %d", resp.StatusCode, body, handlerCalled)
	}

	// 2. First request with Key -> runs handler and caches
	resp, body = doRequest(http.MethodPost, "key123", "user1")
	if resp.StatusCode != http.StatusCreated || body != "success" || handlerCalled != 2 {
		t.Fatalf("expected handler to run normally, got %d, %s, %d", resp.StatusCode, body, handlerCalled)
	}
	if resp.Header.Get("Idempotent-Replay") != "" {
		t.Fatal("first request shouldn't have replay header")
	}

	// 3. Replay with same Key -> uses cache, handler NOT called
	resp, body = doRequest(http.MethodPost, "key123", "user1")
	if resp.StatusCode != http.StatusCreated || body != "success" || handlerCalled != 2 { // Handler not incremented
		t.Fatalf("expected cached response, got %d, %s, calls=%d", resp.StatusCode, body, handlerCalled)
	}
	if resp.Header.Get("Idempotent-Replay") != "true" {
		t.Fatal("expected replay header")
	}
	if resp.Header.Get("X-Custom") != "test" {
		t.Fatal("expected custom header from cache")
	}

	// 4. Different User -> runs handler
	resp, _ = doRequest(http.MethodPost, "key123", "user2")
	if handlerCalled != 3 {
		t.Fatal("expected handler to run for different user")
	}

	// 5. GET request with Key -> ignores idempotency entirely
	resp, _ = doRequest(http.MethodGet, "key123", "user1")
	if handlerCalled != 4 {
		t.Fatal("expected handler to run for GET request")
	}
}
