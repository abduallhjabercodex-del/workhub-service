# Day 20 System Tradeoffs & Architectural Defense

**Project:** WorkHub Enterprise Modular Monolith  
**Evaluation Phase:** Day 20 Final Qualification (Weekly Task 4)  
**Author:** Lead Backend Architect  
**Date:** 2026-09-22  

---

## 1. Architectural Overview & Design Philosophy

WorkHub is engineered as a **High-Resilience Modular Monolith** in Go. Rather than prematurely fragmenting the domain into microservices, WorkHub enforces strict internal architectural boundaries (`handler` $\rightarrow$ `service` $\rightarrow$ `repository` $\rightarrow$ `domain`) mediated exclusively by Go interfaces.

This document articulates and defends the foundational engineering tradeoffs, comparing our chosen design patterns against rejected alternatives.

```text
               +-------------------------------------------------------------+
               |                  NGINX Reverse Proxy (8090)                 |
               +------------------------------+------------------------------+
                                              |
                                              v
+-----------------------------------------------------------------------------------------+
|                                  WorkHub API (Go 1.22+)                                 |
|                                                                                         |
|  [Security Chain: Recovery -> RequestID -> RequestLog -> Metrics -> Timeout -> CORS]    |
|                                                                                         |
|  +--------------------+   +---------------------+   +--------------------------------+  |
|  |   Auth Middleware  |   | Rate Limiting Store |   | Load Shedding (Max In-Flight)  |  |
|  +---------+----------+   +----------+----------+   +---------------+----------------+  |
|            |                         |                              |                   |
|            +-------------------------+------------------------------+                   |
|                                      |                                                  |
|                                      v                                                  |
|  +-----------------------------------------------------------------------------------+  |
|  |                            Business Service Layer                                 |  |
|  |     (Auth Service | Project Service | Task Service | Quota / Membership Service)   |  |
|  +---------+-------------------------+------------------------------+----------------+  |
|            |                         |                              |                   |
|            | Singleflight            | Circuit Breaker              | Bulkhead Isolation|
|            v                         v                              v                   |
|  +--------------------+   +---------------------+   +--------------------------------+  |
|  |  Cache-Aside Layer |   |   Outbox Publisher  |   |    PostgreSQL Repository       |  |
|  +---------+----------+   +----------+----------+   +---------------+----------------+  |
+------------|-------------------------|------------------------------|-------------------+
             |                         |                              |
             v                         v                              v
      +--------------+          +--------------+             +------------------+
      |  Redis Cache |          | Redis Broker |             |    PostgreSQL    |
      |   (Port 6379)|          |  (Port 6379) |             |   (ACID Store)   |
      +--------------+          +-------+------+             +------------------+
                                        |
                                        v
                               +----------------+
                               | Worker Daemon  |
                               | (Idempotent DLQ|
                               +----------------+
```

---

## 2. In-Depth Defense of Architectural Tradeoffs

### Tradeoff 1: Modular Monolith vs. Microservices

| Attribute | Chosen: Modular Monolith | Rejected: Microservices Architecture |
|---|---|---|
| **Boundary Enforcement** | Go package privacy, dependency injection, and clean domain interfaces. | Network boundaries (HTTP/gRPC) across distinct container repositories. |
| **Data Consistency** | Local ACID transactions in PostgreSQL. Single database schema. | Distributed transactions, Two-Phase Commit (2PC), or complex Saga orchestrators. |
| **Operational Overhead** | Single container image (`workhub-api`), 1 deployment pipeline, unified logs. | Multiple pipelines, service meshes, distributed tracing overhead, multi-repo drift. |
| **Latency Profile** | Sub-millisecond in-process function calls. | 2–15ms serialization, TLS handshake, and network hop overhead per call. |

* **Architectural Rationale:**
  Microservices solve *organizational scaling* bottlenecks when hundreds of developers work across distinct product boundaries. At our scale, microservices would introduce distributed failure modes, network serialization costs, and eventual consistency bugs without any tangible benefit. Our interfaces in `internal/domain/interfaces.go` guarantee that any single sub-domain (such as Notifications or Quotas) can be cleanly extracted into an independent microservice if future scale demands it.

---

### Tradeoff 2: Dual-Token Auth with Replay Revocation vs. Stateful Sessions

| Attribute | Chosen: Dual-Token (JWT + Rotating Refresh) | Alternative: Pure Stateful Sessions | Alternative: Pure Stateless JWT |
|---|---|---|---|
| **API Verification Speed** | Instant ($< 0.1\text{ms}$) CPU-bound cryptographic verification. | Requires database / Redis lookup on every single API request. | Fast cryptographic verification. |
| **Revocation Capability** | Refresh rotation revokes entire token family upon detection of token reuse. | Immediate revocation via session key deletion. | Cannot revoke access token until expiration ($15\text{m}$). |
| **Horizontal Scalability** | API nodes scale statelessly without shared session affinity. | Requires sticky sessions or high-throughput distributed session store. | Completely stateless. |

* **Architectural Rationale (ADR-002):**
  We selected **15-minute stateless HMAC-SHA256 JWT access tokens** paired with **cryptographically random, single-use refresh tokens** persisted in PostgreSQL with `family_id` tracking. If an attacker intercepts and attempts to reuse an expired or consumed refresh token, our replay-detection engine immediately revokes all sibling and descendant tokens in that family, locking down the account.

---

### Tradeoff 3: Cache-Aside with Singleflight & Circuit Breaker vs. Write-Through

| Attribute | Chosen: Cache-Aside + Singleflight + Breaker | Rejected: Write-Through Caching |
|---|---|---|
| **Source of Truth** | PostgreSQL remains the sole authoritative source of truth. | Redis and PostgreSQL are both written synchronously. |
| **Cache Stampede Defense** | `singleflight.Group` deduplicates concurrent cold misses into 1 DB query. | Vulnerable to stampedes on cache invalidation spikes. |
| **Degradation Policy** | **Fail-Open**: If Redis fails, Circuit Breaker trips and queries hit DB cleanly. | **Fail-Closed**: Write failure on cache aborts transaction. |

* **Architectural Rationale (ADR-004):**
  A cache must accelerate, never impair, data reliability. By wrapping our Redis read cache in a **Circuit Breaker** (`threshold = 3`, `cooldown = 10s`) and a **Singleflight Group**, 1,000 concurrent requests for the same cold project summary trigger exactly **one** database query, while the remaining 999 requests wait and receive the identical computed value from memory. If Redis crashes, the circuit breaker opens instantly, bypassing Redis and protecting threads from socket timeout stalls.

---

### Tradeoff 4: Strong ACID Consistency vs. Eventual Consistency

| Domain Area | Chosen Consistency Model | Mechanism | Engineering Rationale |
|---|---|---|---|
| **Project Creation & Memberships** | **Strong Consistency** | PostgreSQL serializable/read-committed transactions. | Prevents duplicate owners and unauthorized privilege escalation. |
| **User Quota Enforcement** | **Strong Consistency** | PostgreSQL row-level advisory locks inside transaction. | Zero quota bypasses under 50 concurrent goroutines. |
| **Task Creation Notifications** | **Eventual Consistency** | **Transactional Outbox Pattern** with background worker. | Database commit is never delayed or aborted by broker downtime. |
| **Audit Log Trail** | **Atomic Persistence** | Same transactional scope as business mutation. | Guarantees audit record exists if and only if action committed. |

* **Architectural Rationale (ADR-003):**
  We apply strong consistency where data corruption would be irreversible (money, quotas, roles, ownership) and apply eventual consistency where latency decoupling is vital (notifications, webhooks, analytics).

---

### Tradeoff 5: Layered Resilience (Rate Limit, Bulkhead, Load Shedding)

Rather than relying on a single defense, WorkHub implements defense-in-depth across the request lifecycle:

1. **Edge Rate Limiting (Token Bucket):**
   Throttles client abuse at the entry point (`5 req/12s` for Auth, `10 req/6s` for Projects) returning RFC-standard `429 Too Many Requests` and `Retry-After` headers.
2. **Global Load Shedding (`internal/middleware/loadshed.go`):**
   Guards maximum in-flight requests. If CPU/goroutines approach saturation, excess incoming requests are rejected immediately with `530 In-Flight Limit Exceeded` to preserve server responsiveness for already-queued requests.
3. **Database Concurrency Bulkhead (`internal/bulkhead/`):**
   Isolates heavy analytical and listing queries into a bounded concurrency pool (e.g., maximum 10 simultaneous list-tasks queries). If the pool is saturated, queries fail fast without exhausting the general PostgreSQL connection pool required for authentication and health checks.

---

### Tradeoff 6: Separation of Cache and Message Broker (ADR-001)

* **Decision:** We deploy dedicated infrastructure containers: `workhub_redis` (Port 6379, ephemeral cache) and `workhub_broker` (Port 6379, persistent message queuing).
* **Rationale:** Cache eviction algorithms (e.g., `allkeys-lru`) must never evict pending outbox messages or task notification jobs. In the event of a cache memory spike, transactional event messages remain strictly isolated and durable.
