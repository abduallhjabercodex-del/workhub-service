# WorkHub Authentication & Identity Architecture (Day 11)

This document describes the identity, session, and token lifecycle management implemented in WorkHub.

---

## 🔑 Core Concepts & Architecture

### 1. Credentials vs. Identity
* **Credentials:** The secrets presented during login (e.g. Email + Plaintext Password). Credentials are short-lived in memory and NEVER persisted in plaintext.
* **Identity:** The authenticated user context (`User`, `ID`, `Email`) attached to HTTP requests via `AuthMiddleware`.

### 2. Password Security & Adaptive Hashing
* **Algorithm:** `bcrypt` (`golang.org/x/crypto/bcrypt`).
* **Adaptive Cost Factor:** Configurable via `BCRYPT_COST` (Default: `10`). Adaptive cost ensures password hashing can scale in computation time as hardware performance grows.

---

## 🎟️ Access Tokens & Refresh Tokens Lifecycle

```text
 ┌──────────┐                    ┌────────────────┐                    ┌──────────────┐
 │  Client  │                    │ WorkHub Server │                    │  PostgreSQL  │
 └────┬─────┘                    └───────┬────────┘                    └──────┬───────┘
      │                                  │                                    │
      │ 1. POST /api/v1/auth/register    │                                    │
      │─────────────────────────────────>│ 2. Hash password (bcrypt)          │
      │                                  │ 3. INSERT user                     │
      │                                  │───────────────────────────────────>│
      │                                  │ 4. Save Refresh Token Session      │
      │                                  │───────────────────────────────────>│
      │ 5. Return Access + Refresh Token │                                    │
      │<─────────────────────────────────│                                    │
      │                                  │                                    │
      │ 6. GET /api/v1/auth/me (Bearer)  │                                    │
      │─────────────────────────────────>│ 7. Validate JWT Signature & Exp    │
      │ 8. Return 200 OK (Auth User)     │                                    │
      │<─────────────────────────────────│                                    │
      │                                  │                                    │
      │ 9. POST /api/v1/auth/refresh     │                                    │
      │─────────────────────────────────>│ 10. Verify Non-Revoked & Unexpired │
      │                                  │ 11. Revoke Old + Save New Session  │
      │                                  │───────────────────────────────────>│
      │ 12. Return Rotated Tokens        │                                    │
      │<─────────────────────────────────│                                    │
      │                                  │                                    │
      │ 13. POST /api/v1/auth/logout     │                                    │
      │─────────────────────────────────>│ 14. Mark Revoked (`revoked_at`)    │
      │                                  │───────────────────────────────────>│
      │ 15. Return 204 No Content        │                                    │
      │<─────────────────────────────────│                                    │
```

---

## 🛡️ Security Mechanisms Implemented

### 1. Token Rotation & Reuse Detection
* Every time `/api/v1/auth/refresh` is called, the current Refresh Token is revoked (`revoked_at = NOW()`), and a brand-new Refresh Token is issued.
* **Reuse Detection:** If a previously revoked Refresh Token is presented (indicating a potential token theft/replay attack), the system automatically revokes **ALL** active refresh sessions for that user (`RevokeAllUserRefreshTokens`).

### 2. Token Storage Security
* **Access Tokens:** Signed via HMAC-SHA256 (HS256) JWTs with a short TTL (Default: `15m`).
* **Refresh Tokens:** High-entropy 256-bit random opaque strings. Only the **SHA-256 hash** (`token_hash`) of the refresh token is stored in the database (`refresh_tokens` table), preventing token theft in case of database leaks.

### 3. Log Sanitization & Zero Plaintext Secrets
* Plaintext passwords, raw refresh tokens, and JWT secrets are strictly excluded from structured logs (`slog`).
* `Authorization` headers are sanitized to prevent token exposure in server logs.

---

## 🧪 API Usage Examples

### 1. Register User:
```bash
curl -i -X POST http://127.0.0.1:8080/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"engineer@workhub.dev","displayName":"Lead Dev","password":"Password123!"}'
```

### 2. Login User:
```bash
curl -i -X POST http://127.0.0.1:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"engineer@workhub.dev","password":"Password123!"}'
```

### 3. Get Current Profile (Authenticated Request):
```bash
curl -i http://127.0.0.1:8080/api/v1/auth/me \
  -H "Authorization: Bearer <ACCESS_TOKEN>"
```

### 4. Refresh Access Token (Token Rotation):
```bash
curl -i -X POST http://127.0.0.1:8080/api/v1/auth/refresh \
  -H "Content-Type: application/json" \
  -d '{"refreshToken":"<REFRESH_TOKEN>"}'
```

### 5. Logout & Revoke Session:
```bash
curl -i -X POST http://127.0.0.1:8080/api/v1/auth/logout \
  -H "Content-Type: application/json" \
  -d '{"refreshToken":"<REFRESH_TOKEN>"}'
```
