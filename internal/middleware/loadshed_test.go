package middleware

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
)

func TestLoadShedding(t *testing.T) {
	logger := slog.Default()
	middleware := LoadShedding(2, logger) // max 2 in-flight

	var wg sync.WaitGroup
	var startWg sync.WaitGroup
	blockCh := make(chan struct{})

	var blockedCount atomic.Int32
	targetHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if blockedCount.Add(1) <= 2 {
			startWg.Done()
			<-blockCh // block until test says go
		}
		w.WriteHeader(http.StatusOK)
	})

	handler := middleware(targetHandler)

	// Send 2 requests that will block
	startWg.Add(2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Errorf("expected 200 OK, got %d", rec.Code)
			}
		}()
	}

	// Wait for the 2 requests to reach the handler
	startWg.Wait()

	// 3rd request should fail immediately with 503
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 for 3rd request, got %d", rec.Code)
	}

	// Unblock the first 2 requests
	close(blockCh)
	wg.Wait()

	// 4th request should succeed now that in-flight is 0
	req4 := httptest.NewRequest(http.MethodGet, "/", nil)
	rec4 := httptest.NewRecorder()
	handler.ServeHTTP(rec4, req4)
	if rec4.Code != http.StatusOK {
		t.Fatalf("expected 200 OK after shedding, got %d", rec4.Code)
	}
}
