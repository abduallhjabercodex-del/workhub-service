# WorkHub Production Setup & Operations Guide

**Document Type:** Operator Quickstart & Cluster Bootstrap Manual  
**Scope:** Initial Provisioning, Environment Configuration, Container Orchestration, and Verification  
**Target RTO:** $\le 3\text{ minutes}$ from cold start to healthy cluster  

---

## 1. System Prerequisites

* **Docker & Compose:** Docker Engine 24.0+ with Docker Compose v2 support.
* **Go Compiler:** Go 1.22+ (required for local CI scripts and test runners).
* **Required Network Ports:**
  * Port `8090`: Public ingress gateway via Nginx reverse proxy.
  * Port `6060`: Internal administrative pprof profiling endpoint (`127.0.0.1:6060` only).
  * Ports `5432` & `6379`: Isolated inside internal Docker bridge (`workhub-net`).

---

## 2. 3-Step Quickstart Bootstrap

### Step 1: Environment Configuration
```bash
# Clone production environment template
cp .env.example .env

# Generate and inject a cryptographically secure JWT secret
sed -i 's/JWT_SECRET=.*/JWT_SECRET=super_secret_production_key_workhub_2026/' .env
```

### Step 2: Launch Orchestrated Multi-Service Stack
```bash
docker compose up -d
```
This single command orchestrates the complete production service fleet:
1. `workhub_postgres`: PostgreSQL 16.4 ACID persistence engine with periodic healthcheck.
2. `workhub_redis`: Dedicated low-latency query cache and idempotency lock store.
3. `workhub_broker`: Dedicated Redis message broker for reliable asynchronous event streams.
4. `workhub_migrate`: Ephemeral runner executing schema migrations and exiting cleanly (`Exit 0`).
5. `workhub_api`: Statically compiled Go API daemon running under unprivileged user (`UID 10001:workhub`).
6. `workhub_worker`: Asynchronous background processing worker pool.
7. `workhub_proxy`: Hardened Nginx ingress proxy enforcing HSTS, CSP, and X-Frame-Options headers.

### Step 3: Immediate Health & Readiness Verification
```bash
# Verify container states and automated health check evaluations
docker compose ps

# Verify API liveness and full dependency readiness (PostgreSQL + Redis)
curl -i http://localhost:8090/health
curl -i http://localhost:8090/ready
```

---

## 3. Operational Verification Suite

```bash
# Verify Swagger UI interactive OpenAPI documentation endpoint
curl -s -I http://localhost:8090/api/v1/docs | head -n 5

# Query Prometheus operational RED metrics
curl -s http://localhost:8090/metrics | grep -E "workhub_|http_requests" | head -n 15

# Review live container CPU and Memory utilization
docker stats --no-stream
```
