package middleware

import (
	"fmt"
	"testing"
	"time"

	"pgregory.net/rapid"
)

// =============================================================================
// الخاصية 1: Rate Limiter لا يسمح أبداً بأكثر من الـ capacity
// مهما كان عدد الطلبات
// =============================================================================
func TestProperty_RateLimiterNeverExceedsCapacity(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		capacity := rapid.IntRange(1, 20).Draw(t, "capacity")
		requests := rapid.IntRange(capacity, capacity*3).Draw(t, "requests")

		limiter := NewRateLimiter(capacity, time.Minute) // refill بطيء جداً
		key := "test-user"

		allowed := 0
		for i := 0; i < requests; i++ {
			if limiter.Allow(key) {
				allowed++
			}
		}

		// الخاصية: عدد الطلبات المقبولة لا يتجاوز الـ capacity أبداً
		if allowed > capacity {
			t.Fatalf("allowed=%d exceeded capacity=%d (requests=%d)", allowed, capacity, requests)
		}
	})
}

// =============================================================================
// الخاصية 2: عزل المستخدمين — كل مستخدم له حصته المستقلة
// طلبات user-A لا تؤثر على حصة user-B
// =============================================================================
func TestProperty_RateLimiterUsersAreIsolated(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		capacity := rapid.IntRange(3, 10).Draw(t, "capacity")
		numUsers := rapid.IntRange(2, 5).Draw(t, "numUsers")

		limiter := NewRateLimiter(capacity, time.Minute)

		// استنفد حصة user-0 كاملاً
		for i := 0; i < capacity; i++ {
			limiter.Allow("user:0")
		}

		// الخاصية: المستخدمون الآخرون لا يزالون يملكون حصتهم كاملة
		for u := 1; u < numUsers; u++ {
			key := fmt.Sprintf("user:%d", u)
			if !limiter.Allow(key) {
				t.Fatalf("user:%d was blocked although they haven't used their quota (capacity=%d)", u, capacity)
			}
		}
	})
}

// =============================================================================
// الخاصية 3: Remaining لا يكون أبداً أكبر من capacity
// =============================================================================
func TestProperty_RemainingNeverExceedsCapacity(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		capacity := rapid.IntRange(1, 50).Draw(t, "capacity")
		requests := rapid.IntRange(1, capacity+5).Draw(t, "requests")

		limiter := NewRateLimiter(capacity, time.Second)

		for i := 0; i < requests; i++ {
			_, remaining, cap, _, _ := limiter.AllowWithDetails("user:test")

			// الخاصية: remaining لا تتجاوز capacity أبداً
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
// الخاصية 4: الطلب الأول يُقبل دائماً (إذا كان الـ limiter جديداً)
// =============================================================================
func TestProperty_FirstRequestAlwaysAllowed(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		capacity := rapid.IntRange(1, 100).Draw(t, "capacity")
		key := rapid.StringMatching(`[a-z]{1,20}`).Draw(t, "key")

		// كل مستخدم جديد يجب أن يُقبل في طلبه الأول
		limiter := NewRateLimiter(capacity, time.Minute)

		if !limiter.Allow(key) {
			t.Fatalf("first request for key=%q was denied (capacity=%d)", key, capacity)
		}
	})
}
