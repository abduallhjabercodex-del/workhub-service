package retry

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDo_Success(t *testing.T) {
	cfg := DefaultConfig()
	cfg.BaseDelay = time.Millisecond
	cfg.MaxAttempts = 3

	attempts := 0
	err := Do(context.Background(), cfg, func(ctx context.Context) error {
		attempts++
		if attempts < 2 {
			return errors.New("temp error")
		}
		return nil
	})

	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if attempts != 2 {
		t.Fatalf("expected 2 attempts, got %d", attempts)
	}
}

func TestDo_PermanentError(t *testing.T) {
	cfg := DefaultConfig()
	cfg.BaseDelay = time.Millisecond
	cfg.MaxAttempts = 5

	attempts := 0
	origErr := errors.New("permanent failure")

	err := Do(context.Background(), cfg, func(ctx context.Context) error {
		attempts++
		return Permanent(origErr)
	})

	if !IsPermanent(err) {
		t.Fatalf("expected permanent error, got %v", err)
	}
	if attempts != 1 {
		t.Fatalf("expected 1 attempt, got %d", attempts)
	}
}

func TestDo_MaxAttemptsReached(t *testing.T) {
	cfg := DefaultConfig()
	cfg.BaseDelay = time.Millisecond
	cfg.MaxAttempts = 3
	cfg.JitterFactor = 0 // Disable jitter for predictable test

	attempts := 0
	expectedErr := errors.New("always fails")

	err := Do(context.Background(), cfg, func(ctx context.Context) error {
		attempts++
		return expectedErr
	})

	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected expectedErr, got %v", err)
	}
	if attempts != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts)
	}
}

func TestDo_ContextCancellation(t *testing.T) {
	cfg := DefaultConfig()
	cfg.BaseDelay = 50 * time.Millisecond
	cfg.MaxAttempts = 10

	ctx, cancel := context.WithCancel(context.Background())

	attempts := 0
	err := Do(ctx, cfg, func(ctx context.Context) error {
		attempts++
		if attempts == 2 {
			cancel() // Cancel during the 2nd attempt
		}
		return errors.New("temp error")
	})

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	// It should stop trying after cancellation
	if attempts > 2 {
		t.Fatalf("expected max 2 attempts, got %d", attempts)
	}
}

func TestBackoffAndJitterGrowth(t *testing.T) {
	// We want to test that backoff grows exponentially and jitter works.
	// Since Do() sleeps, we don't want to actually sleep a long time.
	// We can't easily mock time here unless we inject a sleeper, but we can verify
	// the formula logic if we had exported it. For now, we just test with a small delay.
	cfg := Config{
		MaxAttempts:  4,
		BaseDelay:    10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		JitterFactor: 0.3,
	}

	start := time.Now()
	_ = Do(context.Background(), cfg, func(ctx context.Context) error {
		return errors.New("fail")
	})
	duration := time.Since(start)

	// Attempts: 1, 2, 3, 4
	// Delays: 10ms, 20ms, 40ms
	// Sum of delays: ~70ms
	// With 30% jitter, min is 70 - 21 = 49ms, max is 70 + 21 = 91ms.
	if duration < 40*time.Millisecond || duration > 200*time.Millisecond {
		t.Errorf("expected duration roughly around 70ms, got %v", duration)
	}
}
