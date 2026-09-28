# ADR-002: Dual-Token Authentication with Cryptographic Replay Attack Detection

* **Status:** Accepted
* **Date:** 2026-09-18
* **Decision Makers:** Security Engineering, Backend Architecture

---

## Context
Relying exclusively on long-lived bearer tokens exposes user accounts to prolonged unauthorized access if a token is intercepted. Conversely, rolling short-lived tokens requires a refresh mechanism that must defend against stolen refresh tokens being replayed over the wire.

## Decision
Implement a hardened dual-token architecture with single-use refresh token rotation:
1. **Short-Lived Access Token:** 15-minute expiration, stateless JWT cryptographically signed via `HMAC-SHA256`.
2. **Opaque Refresh Token:** 7-day expiration, stored in PostgreSQL as a cryptographic hash (`SHA-256`) with associated session lineage metadata.
3. **Single-Use Rotation:** Every token refresh operation revokes the presented refresh token and issues a new pair.
4. **Replay Attack Revocation:** If a previously consumed or invalidated refresh token is presented again, the system identifies an active replay breach and immediately invalidates the entire session lineage for that user.

## Consequences & Trade-offs
* **Positive:** Absolute protection against persistent session hijacking; immediate detection and automated neutralization of stolen credentials.
* **Negative:** Requires an indexed database query upon each token refresh (acceptable overhead given 15-minute rotation intervals).

## Verification Commands
```bash
go test -v -race ./tests/abuse/credentials_test.go -run="TestInvalidCredentials/Refresh_with_Revoked"
```
