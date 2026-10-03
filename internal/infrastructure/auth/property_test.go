package auth

import (
	"strings"
	"testing"
	"time"

	"pgregory.net/rapid"
)

// =============================================================================
// Property 1: Token Round-Trip
// GenerateAccessToken then ValidateAccessToken -> must always succeed
// regardless of userID or email content
// =============================================================================
func TestProperty_TokenRoundTrip(t *testing.T) {
	const secret = "test-secret-key-that-is-32-bytes-long!"

	rapid.Check(t, func(t *rapid.T) {
		// rapid generates random userID on each iteration
		userID := rapid.StringMatching(`[a-zA-Z0-9\-_]{1,50}`).Draw(t, "userID")
		email := rapid.StringMatching(`[a-z]{1,20}@[a-z]{1,10}\.[a-z]{2,4}`).Draw(t, "email")
		ttlMinutes := rapid.IntRange(1, 60).Draw(t, "ttlMinutes")
		ttl := time.Duration(ttlMinutes) * time.Minute

		// Generate token
		token, err := GenerateAccessToken(userID, email, secret, ttl)
		if err != nil {
			t.Fatalf("GenerateAccessToken failed for userID=%q: %v", userID, err)
		}

		// Invariant: any freshly generated token must validate successfully
		claims, err := ValidateAccessToken(token, secret)
		if err != nil {
			t.Fatalf("ValidateAccessToken failed for userID=%q email=%q: %v", userID, email, err)
		}
		if claims.Sub != userID {
			t.Fatalf("claims.Sub=%q != userID=%q", claims.Sub, userID)
		}
		if claims.Email != email {
			t.Fatalf("claims.Email=%q != email=%q", claims.Email, email)
		}
	})
}

// =============================================================================
// Property 2: HMAC Idempotency (analogous to sort(sort(x)) == sort(x))
// HashToken(x) == HashToken(x) always — same input yields identical hash
// =============================================================================
func TestProperty_HashTokenIdempotent(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		token := rapid.String().Draw(t, "token")

		hash1 := HashToken(token)
		hash2 := HashToken(token)

		// Invariant: identical input must always produce identical output
		if hash1 != hash2 {
			t.Fatalf("HashToken not idempotent for token=%q: got %q then %q", token, hash1, hash2)
		}
	})
}

// =============================================================================
// Property 3: Wrong Secret -> Rejection
// Token generated with secret-A must never validate with secret-B
// =============================================================================
func TestProperty_WrongSecretAlwaysRejected(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		secretA := rapid.StringMatching(`[a-zA-Z0-9]{32,64}`).Draw(t, "secretA")
		secretB := rapid.StringMatching(`[a-zA-Z0-9]{32,64}`).Draw(t, "secretB")

		// Skip when generated secrets happen to match
		if secretA == secretB {
			t.Skip()
		}

		token, err := GenerateAccessToken("user-123", "test@test.com", secretA, time.Hour)
		if err != nil {
			t.Skip() // Skip invalid token generation
		}

		// Invariant: token signed with secretA must be rejected by secretB
		_, err = ValidateAccessToken(token, secretB)
		if err == nil {
			t.Fatalf("token signed with secretA was accepted by secretB! secretA=%q secretB=%q", secretA, secretB)
		}
	})
}

// =============================================================================
// Property 4: Token Structure — must always consist of 3 distinct segments
// =============================================================================
func TestProperty_TokenAlwaysThreeParts(t *testing.T) {
	const secret = "test-secret-key-that-is-32-bytes-long!"

	rapid.Check(t, func(t *rapid.T) {
		userID := rapid.StringMatching(`[a-zA-Z0-9]{1,30}`).Draw(t, "userID")

		token, err := GenerateAccessToken(userID, "x@x.com", secret, time.Hour)
		if err != nil {
			t.Skip()
		}

		// Invariant: JWT must consist of exactly 3 parts separated by "."
		parts := strings.Split(token, ".")
		if len(parts) != 3 {
			t.Fatalf("token has %d parts (want 3): %q", len(parts), token)
		}
		for i, part := range parts {
			if part == "" {
				t.Fatalf("token part[%d] is empty", i)
			}
		}
	})
}

// =============================================================================
// Property 5: Expired Token must always be rejected
// (Uses negative TTL to simulate expired tokens)
// =============================================================================
func TestProperty_ExpiredTokenAlwaysRejected(t *testing.T) {
	const secret = "test-secret-key-that-is-32-bytes-long!"

	rapid.Check(t, func(t *rapid.T) {
		userID := rapid.StringMatching(`[a-zA-Z0-9]{1,30}`).Draw(t, "userID")

		// Generate token with negative TTL (already expired)
		token, err := GenerateAccessToken(userID, "x@x.com", secret, -time.Hour)
		if err != nil {
			t.Skip() // GenerateAccessToken may reject negative TTL
		}

		// Invariant: expired token must always be rejected
		_, err = ValidateAccessToken(token, secret)
		if err == nil {
			t.Fatalf("expired token was accepted for userID=%q", userID)
		}
	})
}
