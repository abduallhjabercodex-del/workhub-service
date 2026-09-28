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

var (
	ErrInvalidToken = errors.New("invalid token")
	ErrExpiredToken = errors.New("token expired")
)

const (
	// Issuer / Audience للتحقق في Multi-Service.
	TokenIssuer   = "workhub"
	TokenAudience = "workhub-api"
	// ClockSkew للتسامح مع اختلاف الساعات.
	clockSkew = 60 * time.Second
)

type JWTClaims struct {
	Sub   string `json:"sub"`
	Email string `json:"email"`
	Iat   int64  `json:"iat"` // ← يُضبط الآن بشكل صحيح
	Exp   int64  `json:"exp"`
	Iss   string `json:"iss,omitempty"`
	Aud   string `json:"aud,omitempty"`
}

type jwtHeader struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
}

func GenerateAccessToken(userID string, email string, secret string, ttl time.Duration) (string, error) {
	if userID == "" || secret == "" {
		return "", ErrInvalidToken
	}
	now := time.Now().UTC()
	claims := JWTClaims{
		Sub:   userID,
		Email: email,
		Iat:   now.Unix(), // ← الإصلاح الحرج
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

func ValidateAccessToken(tokenStr string, secret string) (*JWTClaims, error) {
	parts := strings.Split(tokenStr, ".")
	if len(parts) != 3 {
		return nil, ErrInvalidToken
	}

	// 1) التحقق من الـ header أولاً (يمنع Algorithm Confusion)
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, ErrInvalidToken
	}
	var header jwtHeader
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return nil, ErrInvalidToken
	}
	if header.Alg != "HS256" {
		return nil, ErrInvalidToken // ← منع alg=none و RS256 confusion
	}

	// 2) التحقق من التوقيع
	unsignedToken := parts[0] + "." + parts[1]
	expectedSig := signHMAC256(unsignedToken, secret)
	if !hmac.Equal([]byte(parts[2]), []byte(expectedSig)) {
		return nil, ErrInvalidToken
	}

	// 3) فك الـ claims
	claimsBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, ErrInvalidToken
	}
	var claims JWTClaims
	if err := json.Unmarshal(claimsBytes, &claims); err != nil {
		return nil, ErrInvalidToken
	}

	// 4) التحقق من الحقول الإلزامية
	if claims.Sub == "" || claims.Exp == 0 {
		return nil, ErrInvalidToken
	}
	if claims.Iat == 0 || claims.Iat > time.Now().UTC().Add(clockSkew).Unix() {
		return nil, ErrInvalidToken
	}

	// 5) التحقق من Iss / Aud (إن وُجدا)
	if claims.Iss != "" && claims.Iss != TokenIssuer {
		return nil, ErrInvalidToken
	}
	if claims.Aud != "" && claims.Aud != TokenAudience {
		return nil, ErrInvalidToken
	}

	// 6) التحقق من الانتهاء
	if time.Now().UTC().Unix() > claims.Exp {
		return nil, ErrExpiredToken
	}

	return &claims, nil
}

func GenerateOpaqueToken() (tokenStr string, hashStr string, err error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", "", err
	}
	tokenStr = base64.RawURLEncoding.EncodeToString(bytes)
	hashStr = HashToken(tokenStr)
	return tokenStr, hashStr, nil
}

func HashToken(tokenStr string) string {
	h := sha256.Sum256([]byte(tokenStr))
	return hex.EncodeToString(h[:])
}

func signHMAC256(message string, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(message))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
