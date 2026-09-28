package circuitbreaker

import (
	"testing"
	"time"
)

func TestBreaker_StateTransitions(t *testing.T) {
	now := time.Now()

	// Create a breaker with a threshold of 2 and 1-second cooldown
	b := New(2, time.Second)

	// Inject a controlled time function for deterministic testing
	b.timeNow = func() time.Time { return now }

	// Initial state should be Closed
	if state := b.State(); state != StateClosed {
		t.Fatalf("expected state %s, got %s", StateClosed, state)
	}

	// First failure - should remain Closed
	b.Record(false)
	if state := b.State(); state != StateClosed {
		t.Fatalf("expected state %s after 1 failure, got %s", StateClosed, state)
	}
	if err := b.Allow(); err != nil {
		t.Fatalf("expected Allow to be nil, got %v", err)
	}

	// Second failure - should hit threshold and open
	b.Record(false)
	if state := b.State(); state != StateOpen {
		t.Fatalf("expected state %s after 2 failures, got %s", StateOpen, state)
	}
	if err := b.Allow(); err != ErrOpen {
		t.Fatalf("expected Allow to return ErrOpen, got %v", err)
	}

	// Advance time past the cooldown
	now = now.Add(2 * time.Second)

	// State should now be HalfOpen
	if state := b.State(); state != StateHalfOpen {
		t.Fatalf("expected state %s after cooldown, got %s", StateHalfOpen, state)
	}
	if err := b.Allow(); err != nil {
		t.Fatalf("expected Allow to be nil during HalfOpen, got %v", err)
	}

	// Success in HalfOpen should transition back to Closed
	b.Record(true)
	b.Record(true)
	b.Record(true)
	if state := b.State(); state != StateClosed {
		t.Fatalf("expected state %s after success, got %s", StateClosed, state)
	}

	// Fail again to test HalfOpen -> Open
	b.Record(false)                // 1
	b.Record(false)                // 2 -> Open
	now = now.Add(2 * time.Second) // -> HalfOpen

	// Fail during HalfOpen
	b.Record(false)
	if state := b.State(); state != StateOpen {
		t.Fatalf("expected state %s after failure in HalfOpen, got %s", StateOpen, state)
	}
}
