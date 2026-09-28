# Release Notes — v0.3.0 (Day 15 Deliverable)

**Release Date:** September 19, 2026  
**Tag:** `v0.3.0`  
**Status:** Production Ready  

---

## 🚀 What Was Shipped (Day 15 Audit & Deliverable)

Day 15 deliverable (`v0.3.0`) provides a production-grade, highly resilient project management service with complete RBAC, quota management, rate limiting, and observability:

1. **Role-Based Access Control (RBAC):** Strict role hierarchy (`owner > admin > member > viewer`) with advisory lock quota enforcement for projects, tasks, and members.
2. **Idempotency & Replay Safety:** Redis-backed `Idempotency-Key` middleware caching HTTP responses to prevent duplicate operations.
3. **Resilience Stack:** 3-state circuit breaker for Redis cache, bounded worker pools with backpressure handling (`ErrQueueFull`), database bulkheads, and exponential backoff retries with jitter.
4. **Full Observability Suite:** Prometheus RED/USE metrics, OpenTelemetry distributed tracing, structured JSON logging with context correlation and automated PII redaction.
5. **Operational Probes:** Liveness (`/health`), Readiness (`/ready`), and Startup (`/startup`) HTTP probes. `/health` exposes release version `v0.3.0`.

---

## 📋 Migration Guide

### Database Schema (PostgreSQL)
All migrations (`0001_initial_schema.up.sql`, `0002_quotas_and_audit.up.sql`, `0003_outbox.up.sql`) must be applied prior to deployment:

```bash
migrate -path db/migrations -database "$DATABASE_URL" up
```

Key schema elements introduced:
- `projects.deleted_at` soft deletion column.
- `refresh_token_sessions` table with `used_at` timestamp tracking for token replay detection.
- Transactional `outbox` table for event publication.

---

## 🛠️ Upgrade & Deployment Instructions

### 1. Build Binary
Set commit hash and version flags during compile time:

```bash
COMMIT_HASH=$(git rev-parse --short HEAD)
go build -ldflags "-X workhub-service/internal/version.Version=v0.3.0 -X workhub-service/internal/version.Commit=${COMMIT_HASH}" -o bin/server cmd/server/main.go
```

### 2. Required Environment Variables
Ensure the following variables are configured in `.env` or production environment:

```env
APP_ENV=production
HOST=0.0.0.0
PORT=8080
DATABASE_URL=postgres://user:password@localhost:5432/workhub?sslmode=disable
REDIS_URL=redis://localhost:6379/0
JWT_SECRET=your-32-character-minimum-jwt-secret-key!
MAX_PROJECTS_PER_USER=50
WORKER_CONCURRENCY=5
WORKER_QUEUE_SIZE=100
```

### 3. Verify Health
Validate deployment health after startup:

```bash
curl -s http://localhost:8080/health
# Response: {"status":"ok","version":"v0.3.0","time":"..."}

curl -s http://localhost:8080/ready
# Response: {"status":"ok","time":"..."}
```
