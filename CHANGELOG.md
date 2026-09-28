# Changelog — WorkHub Service

All notable changes to the WorkHub project management service will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [v0.3.0] - 2026-09-19 (Day 15 Deliverable)

### 🔒 Security Fixes
- **RBAC Guard Enforcement:** Enforced strict multi-tenant authorization guards (`owner > admin > member > viewer`). Non-members receive HTTP 403 Forbidden on resource access attempts.
- **Single Resource REST Endpoint Fix:** Added missing `GET /api/v1/projects/{id}` and `DELETE /api/v1/projects/{id}` route handling with owner/member permission checks.
- **Refresh Token Replay Detection:** Implemented automatic family token revocation upon detection of reused refresh tokens (`ErrTokenReplay`).
- **Algorithm Confusion & Forgery Guard:** Enforced `HS256` header validation rejecting `alg: none` or unexpected asymmetric key algorithms in JWT validation middleware.
- **Log PII & Credential Masking:** Implemented centralized JSON log redactor masking sensitive keys (`password`, `token`, `secret`, `authorization`, `credit_card`, `email`).

### 🛡️ Resilience Improvements
- **3-State Circuit Breaker:** Integrated circuit breaker (`Closed` → `Open` → `HalfOpen`) for Redis operations, providing instant fail-open fallthrough for project summary caches.
- **Rate Limiter Protection:** Enforced token bucket rate limiters on `/api/v1/auth/login` (5/min) and project creation (10/min) returning HTTP 429 with standard `Retry-After` and `X-RateLimit-*` headers.
- **Database Bulkhead Isolation:** Isolated heavy database queries in bounded concurrency semaphores to prevent connection pool exhaustion.
- **Graceful Shutdown:** Configured SIGTERM signal context handling ensuring in-flight HTTP requests finish before server exit.

### 📊 Observability
- **Prometheus Metrics:** Added RED and USE metrics (`workhub_http_requests_total`, `workhub_db_query_duration_seconds`, `workhub_cache_circuit_state`, `workhub_rate_limit_rejected_total`, `workhub_idempotency_requests_total`).
- **Distributed Tracing:** Instrumented OpenTelemetry HTTP middleware (`otelhttp`) with service and database child span propagation.
- **Health Probes:** Added `/health` (liveness), `/ready` (readiness with DB ping), and `/startup` probes. `/health` response now exposes current release version (`v1.0.0-week1`).

### ⚠️ Breaking Changes
- `POST /api/v1/auth/refresh` now requires JSON key `refreshToken` matching OpenAPI 3.0 specification (`refresh_token` deprecated).
- Project sub-resource URLs follow standardized `/api/v1/projects/{id}/...` path hierarchy.

### 📌 Known Issues
- Single-node dev mode uses in-memory token bucket rate limiting. Production deployments must configure `REDIS_URL` to enable cluster-wide sliding window Lua scripts.
