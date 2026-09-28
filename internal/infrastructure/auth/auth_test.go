package auth

import (
	"testing"
	"time"
)

// TestPasswordHashingAndVerification يختبر تجزئة (hashing) كلمة المرور والتحقق من المطابقة مع كلمة المرور الصحيحة والخاطئة.
func TestPasswordHashingAndVerification(t *testing.T) {
	password := "SecretP@ss123"
	hash, err := HashPassword(password, 10)
	if err != nil {
		t.Fatalf("HashPassword error = %v", err)
	}
	if !CheckPasswordHash(password, hash) {
		t.Fatal("CheckPasswordHash failed for valid password")
	}
	if CheckPasswordHash("WrongPassword", hash) {
		t.Fatal("CheckPasswordHash succeeded for wrong password")
	}
}

// TestJWTGenerationValidationAndExpiry يختبر إنشاء رموز الوصول (JWT Access Token)، والتحقق من بياناتها (claims)، واكتشاف المفاتيح السرية الخاطئة، والتأكد من التعامل مع الرموز المنتهية الصلاحية.
func TestJWTGenerationValidationAndExpiry(t *testing.T) {
	secret := "test-secret-key"
	userID := "101"
	email := "user@example.com"

	token, err := GenerateAccessToken(userID, email, secret, time.Minute)
	if err != nil {
		t.Fatalf("GenerateAccessToken error = %v", err)
	}

	claims, err := ValidateAccessToken(token, secret)
	if err != nil {
		t.Fatalf("ValidateAccessToken error = %v", err)
	}
	if claims.Sub != userID || claims.Email != email {
		t.Fatalf("claims sub=%s email=%s, want %s %s", claims.Sub, claims.Email, userID, email)
	}

	// Invalid Secret
	if _, err := ValidateAccessToken(token, "bad-secret"); err != ErrInvalidToken {
		t.Fatalf("validation with bad secret error = %v, want ErrInvalidToken", err)
	}

	// Expired Token
	expiredToken, err := GenerateAccessToken(userID, email, secret, -time.Second)
	if err != nil {
		t.Fatalf("GenerateAccessToken error = %v", err)
	}
	if _, err := ValidateAccessToken(expiredToken, secret); err != ErrExpiredToken {
		t.Fatalf("validation for expired token error = %v, want ErrExpiredToken", err)
	}
}

// TestGenerateOpaqueTokenAndHash يختبر إنشاء الرموز العتمة (Opaque Refresh Tokens) وتوليد التجزئة الخاص بها (HashToken) والتأكد من مطابقتها.
func TestGenerateOpaqueTokenAndHash(t *testing.T) {
	token, hash, err := GenerateOpaqueToken()
	if err != nil {
		t.Fatalf("GenerateOpaqueToken error = %v", err)
	}
	if token == "" || hash == "" {
		t.Fatal("token or hash is empty")
	}
	if HashToken(token) != hash {
		t.Fatal("HashToken mismatch")
	}
}
