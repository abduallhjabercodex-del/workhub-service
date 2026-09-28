# WorkHub Known Limitations & Architectural Roadmap

**Document Type:** Production Readiness Evaluation & Architectural Boundaries  
**System:** WorkHub Enterprise Modular Monolith  
**Scope:** Current System Constraints, Mitigations, and Planned Enhancements  

---

## 1. Architectural Boundaries & Current Limitations

### 1. In-Memory Token Bucket Rate Limiting
* **Current State:** Rate limiters execute locally in memory per API instance using the Token Bucket algorithm.
* **Operational Impact:** When horizontally scaling across multiple API replicas behind a round-robin load balancer, an aggressive client could distribute requests across replicas, effectively multiplying their rate quota.
* **Roadmap Enhancement:** Migrate to a centralized, distributed sliding-window rate limiter powered by Redis scripts (`Redis-backed Sliding Window Rate Limiter`).

### 2. Maximum Payload & Header Size Ceilings
* **Current State:** The HTTP server strictly enforces a **1 MB limit** on request headers and payloads (`MaxHeaderBytes: 1 << 20`) to defend against memory exhaustion and Slowloris attacks.
* **Operational Impact:** Large binary files or multimedia attachments cannot be uploaded directly through the API body.
* **Roadmap Enhancement:** Implement an asynchronous presigned URL generator allowing direct, encrypted client-to-S3 uploads (`Presigned S3 Upload URLs`).

### 3. In-Memory Broker Fallback Volatility
* **Current State:** When `BROKER_URL` is omitted, the event broker defaults to an in-memory queue for local developer convenience. If the process terminates, unhandled in-memory events are discarded.
* **Operational Mitigation:** The dedicated `workhub_broker` container is strictly mandated in production environments and enforced via `docker-compose.yml`.
* **Roadmap Enhancement:** Enforce transactional outbox durability globally so in-memory fallbacks are prohibited outside unit tests.

### 4. Local OpenTelemetry Trace Export
* **Current State:** Distributed traces are recorded locally to memory and log files for developer inspection and benchmarking.
* **Roadmap Enhancement:** Introduce an `OpenTelemetry Collector` sidecar daemon to ship spans to distributed trace backends (Grafana Tempo, Jaeger, or AWS X-Ray).

---

## 2. Constraint Verification Suite

```bash
# 1. Verify HTTP payload size constraint enforcement in API test suite
go test -v ./internal/handler/authz_test.go -run="TestPayloadSizeLimiter"

# 2. Verify that Redis broker and cache endpoints are configured in production
docker compose exec api env | grep -E "BROKER_URL|REDIS_URL"
```
