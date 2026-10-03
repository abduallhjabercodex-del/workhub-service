package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// ─────────────────────────────────────────────────────────────
// Package-level errors
// ─────────────────────────────────────────────────────────────

var (
	ErrInvalidToken = errors.New("invalid token")
	ErrExpiredToken = errors.New("token expired")
)

// ─────────────────────────────────────────────────────────────
// Constants
// ─────────────────────────────────────────────────────────────

const (
	// TokenIssuer and TokenAudience are used for multi-service
	// claims verification.
	TokenIssuer   = "workhub"
	TokenAudience = "workhub-api"

	// clockSkew tolerates minor server clock drift when validating
	// the iat (issued-at) claim.
	clockSkew = 60 * time.Second
)

// ─────────────────────────────────────────────────────────────
// Types
// ─────────────────────────────────────────────────────────────

// JWTClaims represents the payload of an access token.
type JWTClaims struct {
	Sub   string `json:"sub"`           // Subject (user ID)
	Email string `json:"email"`         // User email
	Iat   int64  `json:"iat"`           // Issued-at (Unix seconds)
	Exp   int64  `json:"exp"`           // Expiry (Unix seconds)
	Iss   string `json:"iss,omitempty"` // Issuer
	Aud   string `json:"aud,omitempty"` // Audience
}

// jwtHeader is the fixed JOSE header for HS256 tokens.
type jwtHeader struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
}

// ─────────────────────────────────────────────────────────────
// Access token — generation
// ─────────────────────────────────────────────────────────────

// GenerateAccessToken creates a signed HS256 JWT for the given user.
// Returns ErrInvalidToken if userID or secret is empty.
func GenerateAccessToken(userID string, email string, secret string, ttl time.Duration) (string, error) {
	if userID == "" || secret == "" {
		return "", ErrInvalidToken
	}

	now := time.Now().UTC()
	claims := JWTClaims{
		Sub:   userID,
		Email: email,
		Iat:   now.Unix(), // store issuance time for skew validation
		Exp:   now.Add(ttl).Unix(),
		Iss:   TokenIssuer,
		Aud:   TokenAudience,
	}

	header := jwtHeader{Alg: "HS256", Typ: "JWT"}
	headerBytes, err := json.Marshal(header)
	if err != nil {
		return "", err
	}

	claimsBytes, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}

	encodedHeader := base64.RawURLEncoding.EncodeToString(headerBytes)
	encodedClaims := base64.RawURLEncoding.EncodeToString(claimsBytes)

	unsignedToken := encodedHeader + "." + encodedClaims
	sig := signHMAC256(unsignedToken, secret)

	return unsignedToken + "." + sig, nil
}

// ─────────────────────────────────────────────────────────────
// Access token — validation
// ─────────────────────────────────────────────────────────────

// ValidateAccessToken parses and verifies an HS256 JWT.
//
// Checks performed, in order:
//   1. Structural validity (three dot-separated parts)
//   2. Header algorithm allowlist — mitigates alg=none and
//      asymmetric-algorithm confusion attacks
//   3. HMAC-SHA256 signature (constant-time comparison)
//   4. Claims decoding
//   5. Mandatory claims (sub, exp, iat with skew tolerance)
//   6. Issuer and audience (only if present)
//   7. Expiration timestamp
//
// Returns ErrInvalidToken on any structural or claim violation,
// and ErrExpiredToken only for well-formed but expired tokens.
func ValidateAccessToken(tokenStr string, secret string) (*JWTClaims, error) {
	// 1) Structure: exactly three parts
	parts := strings.Split(tokenStr, ".")
	if len(parts) != 3 {
		return nil, ErrInvalidToken
	}

	// 2) Header: enforce HS256
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, ErrInvalidToken
	}
	var header jwtHeader
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return nil, ErrInvalidToken
	}
	if header.Alg != "HS256" {
		return nil, ErrInvalidToken
	}

	// 3) Signature: constant-time HMAC-SHA256 comparison
	unsignedToken := parts[0] + "." + parts[1]
	expectedSig := signHMAC256(unsignedToken, secret)
	if !hmac.Equal([]byte(parts[2]), []byte(expectedSig)) {
		return nil, ErrInvalidToken
	}

	// 4) Claims: decode
	claimsBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, ErrInvalidToken
	}
	var claims JWTClaims
	if err := json.Unmarshal(claimsBytes, &claims); err != nil {
		return nil, ErrInvalidToken
	}

	// 5) Mandatory claims
	if claims.Sub == "" || claims.Exp == 0 {
		return nil, ErrInvalidToken
	}
	if claims.Iat == 0 || claims.Iat > time.Now().UTC().Add(clockSkew).Unix() {
		return nil, ErrInvalidToken
	}

	// 6) Issuer and audience (verify only if present)
	if claims.Iss != "" && claims.Iss != TokenIssuer {
		return nil, ErrInvalidToken
	}
	if claims.Aud != "" && claims.Aud != TokenAudience {
		return nil, ErrInvalidToken
	}

	// 7) Expiration
	if time.Now().UTC().Unix() > claims.Exp {
		return nil, ErrExpiredToken
	}

	return &claims, nil
}

// ─────────────────────────────────────────────────────────────
// Opaque tokens (refresh tokens, API keys)
// ─────────────────────────────────────────────────────────────

// GenerateOpaqueToken creates a 32-byte random token and its SHA-256 hash.
// The plaintext is returned to the caller (to deliver to the user);
// the hash is what gets stored server-side for later verification.
func GenerateOpaqueToken() (tokenStr string, hashStr string, err error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", "", err
	}
	tokenStr = base64.RawURLEncoding.EncodeToString(bytes)
	hashStr = HashToken(tokenStr)
	return tokenStr, hashStr, nil
}

// HashToken returns the hex-encoded SHA-256 of a token string.
// Used for at-rest storage of opaque tokens.
func HashToken(tokenStr string) string {
	h := sha256.Sum256([]byte(tokenStr))
	return hex.EncodeToString(h[:])
}

// ─────────────────────────────────────────────────────────────
// Internal helpers
// ─────────────────────────────────────────────────────────────

// signHMAC256 returns the base64url-encoded HMAC-SHA256 of message.
func signHMAC256(message string, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(message))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}