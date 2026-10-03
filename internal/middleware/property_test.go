package middleware

import (
	"fmt"
	"testing"
	"time"

	"pgregory.net/rapid"
)

// =============================================================================
// Property 1: Rate Limiter never permits more than capacity
// regardless of request burst volume
// =============================================================================
func TestProperty_RateLimiterNeverExceedsCapacity(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		capacity := rapid.IntRange(1, 20).Draw(t, "capacity")
		requests := rapid.IntRange(capacity, capacity*3).Draw(t, "requests")

		limiter := NewRateLimiter(capacity, time.Minute) // very slow refill
		key := "test-user"

		allowed := 0
		for i := 0; i < requests; i++ {
			if limiter.Allow(key) {
				allowed++
			}
		}

		// Invariant: allowed requests must never exceed capacity
		if allowed > capacity {
			t.Fatalf("allowed=%d exceeded capacity=%d (requests=%d)", allowed, capacity, requests)
		}
	})
}

// =============================================================================
// Property 2: User Isolation — each user maintains independent quota
// Requests by user-A do not affect user-B quota
// =============================================================================
func TestProperty_RateLimiterUsersAreIsolated(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		capacity := rapid.IntRange(3, 10).Draw(t, "capacity")
		numUsers := rapid.IntRange(2, 5).Draw(t, "numUsers")

		limiter := NewRateLimiter(capacity, time.Minute)

		// Exhaust user:0 quota entirely
		for i := 0; i < capacity; i++ {
			limiter.Allow("user:0")
		}

		// Invariant: other users still have full capacity
		for u := 1; u < numUsers; u++ {
			key := fmt.Sprintf("user:%d", u)
			if !limiter.Allow(key) {
				t.Fatalf("user:%d was blocked although they haven't used their quota (capacity=%d)", u, capacity)
			}
		}
	})
}

// =============================================================================
// Property 3: Remaining tokens never exceed capacity
// =============================================================================
func TestProperty_RemainingNeverExceedsCapacity(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		capacity := rapid.IntRange(1, 50).Draw(t, "capacity")
		requests := rapid.IntRange(1, capacity+5).Draw(t, "requests")

		limiter := NewRateLimiter(capacity, time.Second)

		for i := 0; i < requests; i++ {
			_, remaining, cap, _, _ := limiter.AllowWithDetails("user:test")

			// Invariant: remaining never exceeds capacity and is non-negative
			if remaining > cap {
				t.Fatalf("remaining=%d > capacity=%d after %d requests", remaining, cap, i+1)
			}
			if remaining < 0 {
				t.Fatalf("remaining=%d is negative after %d requests", remaining, i+1)
			}
		}
	})
}

// =============================================================================
// Property 4: First request is always allowed for new limiter keys
// =============================================================================
func TestProperty_FirstRequestAlwaysAllowed(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		capacity := rapid.IntRange(1, 100).Draw(t, "capacity")
		key := rapid.StringMatching(`[a-z]{1,20}`).Draw(t, "key")

		// Any new client key must be accepted on first request
		limiter := NewRateLimiter(capacity, time.Minute)

		if !limiter.Allow(key) {
			t.Fatalf("first request for key=%q was denied (capacity=%d)", key, capacity)
		}
	})
}
