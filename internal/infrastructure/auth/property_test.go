package auth

import (
	"strings"
	"testing"
	"time"

	"pgregory.net/rapid"
)

// =============================================================================
// الخاصية 1: Token Round-Trip
// GenerateAccessToken ثم ValidateAccessToken → يجب أن ينجح دائماً
// مهما كان userID أو email
// =============================================================================
func TestProperty_TokenRoundTrip(t *testing.T) {
	const secret = "test-secret-key-that-is-32-bytes-long!"

	rapid.Check(t, func(t *rapid.T) {
		// rapid يُولّد userID عشوائي في كل تشغيل
		userID := rapid.StringMatching(`[a-zA-Z0-9\-_]{1,50}`).Draw(t, "userID")
		email := rapid.StringMatching(`[a-z]{1,20}@[a-z]{1,10}\.[a-z]{2,4}`).Draw(t, "email")
		ttlMinutes := rapid.IntRange(1, 60).Draw(t, "ttlMinutes")
		ttl := time.Duration(ttlMinutes) * time.Minute

		// أنشئ توكن
		token, err := GenerateAccessToken(userID, email, secret, ttl)
		if err != nil {
			t.Fatalf("GenerateAccessToken failed for userID=%q: %v", userID, err)
		}

		// الخاصية: أي توكن مُنشأ يجب أن يُتحقَّق منه بنجاح
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
// الخاصية 2: HMAC Idempotency (مثل sort(sort(x)) == sort(x))
// HashToken(x) == HashToken(x) دائماً — نفس المدخل → نفس الـ hash
// =============================================================================
func TestProperty_HashTokenIdempotent(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		token := rapid.String().Draw(t, "token")

		hash1 := HashToken(token)
		hash2 := HashToken(token)

		// الخاصية: نفس المدخل يجب أن يُعطي نفس النتيجة دائماً
		if hash1 != hash2 {
			t.Fatalf("HashToken not idempotent for token=%q: got %q then %q", token, hash1, hash2)
		}
	})
}

// =============================================================================
// الخاصية 3: Wrong Secret → Rejection
// توكن مُنشأ بـ secret-A لا يجوز قبوله بـ secret-B أبداً
// =============================================================================
func TestProperty_WrongSecretAlwaysRejected(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		secretA := rapid.StringMatching(`[a-zA-Z0-9]{32,64}`).Draw(t, "secretA")
		secretB := rapid.StringMatching(`[a-zA-Z0-9]{32,64}`).Draw(t, "secretB")

		// إذا كان السرّان متطابقَين نتجاهل هذه الحالة
		if secretA == secretB {
			t.Skip()
		}

		token, err := GenerateAccessToken("user-123", "test@test.com", secretA, time.Hour)
		if err != nil {
			t.Skip() // مدخل غير صالح، نتجاهل
		}

		// الخاصية: توكن مُوقَّع بـ secretA يجب أن يُرفض بـ secretB
		_, err = ValidateAccessToken(token, secretB)
		if err == nil {
			t.Fatalf("token signed with secretA was accepted by secretB! secretA=%q secretB=%q", secretA, secretB)
		}
	})
}

// =============================================================================
// الخاصية 4: Token Structure — يجب أن يتكوّن من 3 أجزاء دائماً
// =============================================================================
func TestProperty_TokenAlwaysThreeParts(t *testing.T) {
	const secret = "test-secret-key-that-is-32-bytes-long!"

	rapid.Check(t, func(t *rapid.T) {
		userID := rapid.StringMatching(`[a-zA-Z0-9]{1,30}`).Draw(t, "userID")

		token, err := GenerateAccessToken(userID, "x@x.com", secret, time.Hour)
		if err != nil {
			t.Skip()
		}

		// الخاصية: JWT يجب أن يتكوّن من 3 أجزاء مفصولة بـ "." دائماً
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
// الخاصية 5: Expired Token يجب أن يُرفض دائماً
// (نستخدم TTL سلبي لمحاكاة انتهاء الصلاحية)
// =============================================================================
func TestProperty_ExpiredTokenAlwaysRejected(t *testing.T) {
	const secret = "test-secret-key-that-is-32-bytes-long!"

	rapid.Check(t, func(t *rapid.T) {
		userID := rapid.StringMatching(`[a-zA-Z0-9]{1,30}`).Draw(t, "userID")

		// أنشئ توكن بـ TTL سلبي (منتهي الصلاحية فوراً)
		token, err := GenerateAccessToken(userID, "x@x.com", secret, -time.Hour)
		if err != nil {
			t.Skip() // GenerateAccessToken قد ترفض TTL سلبي
		}

		// الخاصية: توكن منتهي الصلاحية يجب أن يُرفض دائماً
		_, err = ValidateAccessToken(token, secret)
		if err == nil {
			t.Fatalf("expired token was accepted for userID=%q", userID)
		}
	})
}
