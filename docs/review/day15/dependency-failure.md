# Dependency Failure Matrix — WorkHub Service

**Document Status:** Final  
**Date:** 2026-09-19  
**Version:** v1.0.0-week1  
**Scope:** Resilience, Circuit Breakers, Bulkhead Isolation, Fail-Open vs Fail-Closed Strategies.

---

## 1. Overview

WorkHub relies on external infrastructure components (PostgreSQL, Redis, OpenTelemetry Collector, External Webhooks). To prevent cascading failures and guarantee high availability, each dependency interaction is wrapped with resilience patterns (circuit breakers, rate limiters, bulkheads, exponential retries, and fallback strategies).

---

## 2. Dependency Failure Matrix

| Dependency              | Failure Mode                           | System Behavior & Mitigation                                                                                                                                   | Resilience Strategy                                              | Verified?                                          |
| :---------------------- | :------------------------------------- | :------------------------------------------------------------------------------------------------------------------------------------------------------------- | :--------------------------------------------------------------- | :------------------------------------------------- |
| **PostgreSQL**          | Connection Refused / Unreachable       | Startup fails in `production` mode; falls back to `memory` repository in `development` mode. Readiness probe (`/ready`) returns `503 Service Unavailable`.     | Fail-Fast at Edge / Probe Degradation                            | **Yes** (`tests/dependency/db_failure_test.go`)    |
| **PostgreSQL**          | Slow Queries / High Latency            | Request contexts timeout at `REQUEST_TIMEOUT` (e.g. 15s). Heavy DB operations are isolated inside Bulkhead worker pools and reject excess requests with `503`. | Timeout + Bulkhead Isolation                                     | **Yes**                                            |
| **PostgreSQL**          | Connection Pool Exhausted              | New requests queue up to pool timeout, then fail with `503 Service Unavailable` via Bulkhead/DB error mapping.                                                 | Connection Pool Bounding (`MaxConns: 25`)                        | **Yes**                                            |
| **Redis**               | Connection Refused / Unreachable       | Cache reads fail-open (serving database query directly). Rate limiter fails-closed to prevent resource abuse. Circuit breaker opens after 5 failures.          | Circuit Breaker (3-State) + Fail-Open Cache / Fail-Closed Limits | **Yes** (`tests/dependency/redis_failure_test.go`) |
| **Redis**               | Slow Responses / Latency Spikes        | Redis operation context timeout triggers circuit breaker failure count. After 5 timeouts, circuit transitions to `OPEN` (State = 2) for 10s cooldown.          | Circuit Breaker Timeout Protection                               | **Yes**                                            |
| **Redis**               | Partial Outage / Eviction              | Cache misses fall through to database; idempotency store falls back to non-blocking request execution.                                                         | Fail-Open Cache Fallthrough                                      | **Yes**                                            |
| **OTel Collector**      | Collector Unreachable                  | OTLP exporter operates in non-blocking async mode. Failed trace exports log warnings without blocking HTTP response handling.                                  | Non-Blocking Async Exporter                                      | **Yes**                                            |
| **Event Broker**        | In-Memory Channel Full                 | Event publisher returns `ErrQueueFull` under backpressure; worker pool moves unprocessable events to dead-letter queue.                                        | Bounded Buffer + Dead-Letter Queue                               | **Yes** (`internal/worker/pool_test.go`)           |
| **Notification Worker** | External API Down (HTTP 5xx / Timeout) | Worker retries delivery using exponential backoff with random jitter (max 3 attempts). Exhausted retries route to Dead-Letter Store.                           | Retry + Exponential Backoff + Jitter + DLQ                       | **Yes** (`internal/worker/notification_test.go`)   |

---

## 3. Resilience Pattern Deep Dives

### 3.1. Circuit Breaker (Redis Protection)
- **Configured Thresholds:** 5 consecutive failures, 10-second cooldown window (`StateClosed` → `StateOpen` → `StateHalfOpen`).
- **Telemetry:** Exposes `workhub_cache_circuit_state` Prometheus gauge (`0=Closed`, `1=Half-Open`, `2=Open`).
- **Verification Test:** `tests/dependency/redis_failure_test.go` verifies that closing the Redis server transitions the circuit state to `OPEN` (state 2) while HTTP responses continue returning `200 OK` via cache-miss fallback.

### 3.2. Database Bulkhead Isolation
- **Concurrency Boundary:** Heavy analytical / query workloads execute inside a semaphored bulkhead preventing connection pool saturation.
- **Fail-Fast:** Over-capacity requests immediately return HTTP `503 Service Unavailable` with `Retry-After` header.

### 3.3. Readiness & Liveness Probes
- **Liveness Probe (`GET /health`):** Always returns `200 OK` as long as HTTP process loop is alive.
- **Readiness Probe (`GET /ready`):** Evaluates database `Ping(ctx)`. Returns `503 Service Unavailable` if database connection drops, preventing load balancers from routing traffic to degraded nodes.

---

## 4. Evidence Verification Summary

```
+------------------------+------------------------------------+-------------------------+
| Test Suite             | Target Scenario                    | Status                  |
+------------------------+------------------------------------+-------------------------+
| db_failure_test.go     | PostgreSQL Outage & Probes         | PASS                    |
| redis_failure_test.go  | Redis Outage & Circuit Breaker     | PASS                    |
| pool_test.go           | Worker Pool Backpressure           | PASS                    |
| notification_test.go   | External Webhook Failures & DLQ    | PASS                    |
+------------------------+------------------------------------+-------------------------+
```
