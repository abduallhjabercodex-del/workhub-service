package abuse_test

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"workhub-service/internal/middleware"
)

// TestTimeoutBehavior tests timeout enforcement (503), context cancellation, and graceful shutdown.
func TestTimeoutBehavior(t *testing.T) {
	t.Run("Slow request hits timeout -> returns 503", func(t *testing.T) {
		slowHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-time.After(500 * time.Millisecond):
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("done"))
			case <-r.Context().Done():
				// Context cancelled properly by timeout handler
				return
			}
		})

		// 100ms timeout
		handler := middleware.RequestTimeout(100 * time.Millisecond)(slowHandler)
		handler = middleware.Recovery(slog.Default())(handler)

		ts := httptest.NewServer(handler)
		defer ts.Close()

		start := time.Now()
		resp, err := ts.Client().Get(ts.URL)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		duration := time.Since(start)

		if resp.StatusCode != http.StatusServiceUnavailable {
			t.Errorf("expected 503 Service Unavailable on timeout, got %d", resp.StatusCode)
		}

		body, _ := io.ReadAll(resp.Body)
		if len(body) == 0 {
			t.Errorf("expected timeout error message body")
		}

		if duration > 400*time.Millisecond {
			t.Errorf("request took too long to fail: %v", duration)
		}
	})

	t.Run("Context deadline exceeded is handled without panic", func(t *testing.T) {
		deadlineHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), 10*time.Millisecond)
			defer cancel()

			select {
			case <-time.After(100 * time.Millisecond):
				w.WriteHeader(http.StatusOK)
			case <-ctx.Done():
				if ctx.Err() == context.DeadlineExceeded {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusGatewayTimeout)
					_, _ = w.Write([]byte(`{"error":{"code":"gateway_timeout","message":"upstream deadline exceeded"}}`))
					return
				}
			}
		})

		handler := middleware.Recovery(slog.Default())(deadlineHandler)
		ts := httptest.NewServer(handler)
		defer ts.Close()

		resp, err := ts.Client().Get(ts.URL)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusGatewayTimeout && resp.StatusCode != http.StatusServiceUnavailable {
			t.Errorf("expected 504 Gateway Timeout or 503, got %d", resp.StatusCode)
		}
	})

	t.Run("Graceful Shutdown in-flight request completion", func(t *testing.T) {
		inFlightStarted := make(chan struct{})
		inFlightDone := make(chan struct{})

		serverHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			close(inFlightStarted)
			time.Sleep(150 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"success"}`))
			close(inFlightDone)
		})

		srv := &http.Server{
			Addr:    "127.0.0.1:0",
			Handler: serverHandler,
		}

		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("failed to listen: %v", err)
		}

		go func() {
			_ = srv.Serve(listener)
		}()

		// Send request in goroutine
		reqDone := make(chan int, 1)
		go func() {
			resp, err := http.Get("http://" + listener.Addr().String())
			if err == nil {
				reqDone <- resp.StatusCode
				resp.Body.Close()
			} else {
				reqDone <- 0
			}
		}()

		<-inFlightStarted

		// Trigger graceful shutdown while request is in-flight
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		err = srv.Shutdown(shutdownCtx)
		if err != nil {
			t.Errorf("graceful shutdown returned error: %v", err)
		}

		select {
		case statusCode := <-reqDone:
			if statusCode != http.StatusOK {
				t.Errorf("expected in-flight request to finish with 200 OK, got status %d", statusCode)
			}
		case <-time.After(1 * time.Second):
			t.Fatalf("in-flight request did not complete in time")
		}
	})
}
