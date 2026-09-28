# Day 20 Product & Operations Demonstration Runbook

**Project:** WorkHub Enterprise Modular Monolith  
**Evaluation Milestone:** Day 20 Final Qualification  
**Target Audience:** Evaluators, Operations Engineers, System Architects  
**Date:** 2026-09-22  

---

## 1. Demonstration Environment Prerequisites

Before initiating the demonstration, verify that all core infrastructure containers are running and healthy:

```bash
docker compose ps
# Expected: All 7 containers active and healthy:
# workhub_api, workhub_worker, workhub_postgres, workhub_redis, workhub_broker, workhub_prometheus, workhub_grafana, workhub_proxy
```

### Key Demonstration Endpoints:
* **Interactive Swagger UI Portal:** [http://localhost:8090/api/v1/docs](http://localhost:8090/api/v1/docs)
* **Grafana Observability Dashboard:** [http://localhost:3001/d/adg7ldb/workhub-enterprise-observability-and-operations](http://localhost:3001/d/adg7ldb/workhub-enterprise-observability-and-operations) *(Credentials: `admin` / `admin`)*
* **Prometheus Alert Manager / Rules:** [http://localhost:9090/alerts](http://localhost:9090/alerts)
* **Operational Health Probes:** [http://localhost:8090/health](http://localhost:8090/health) & [http://localhost:8090/ready](http://localhost:8090/ready)

---

## 2. Part 1: Product Features Demonstration

### Demo 1.1: Interactive API Exploration via Swagger UI
1. Navigate in browser to `http://localhost:8090/api/v1/docs`.
2. Inspect the **14 fully documented endpoints** across Authentication, Projects, Tasks, and Memberships.
3. Click on `POST /api/v1/auth/login` $\rightarrow$ **Try it out**.
4. Submit default credentials:
   ```json
   {
     "email": "owner@workhub.dev",
     "password": "Password1234!"
   }
   ```
5. Observe the response containing the stateless `accessToken` and rotating `refreshToken`.
6. Copy the `accessToken`, click the green **Authorize** button at the top of Swagger UI, paste the token, and click **Authorize**.

### Demo 1.2: Multi-Tenant RBAC & IDOR Prevention
1. In Swagger UI, execute `GET /api/v1/projects` to list projects accessible to the authenticated user.
2. Attempt to delete or modify a project using a `viewer@workhub.dev` token:
   ```bash
   # Log in as viewer
   VIEWER_TOKEN=$(curl -s -X POST http://localhost:8090/api/v1/auth/login \
     -H "Content-Type: application/json" \
     -d '{"email":"viewer@workhub.dev","password":"Password1234!"}' | grep -o '"accessToken":"[^"]*' | cut -d'"' -f4)

   # Attempt to delete project 3 (Owned by owner@workhub.dev)
   curl -s -w "\nHTTP Status: %{http_code}\n" -X DELETE \
     -H "Authorization: Bearer $VIEWER_TOKEN" \
     http://localhost:8090/api/v1/projects/3
   # Expected Output: HTTP Status: 403 (Forbidden)
   ```

---

## 3. Part 2: Operations & Observability Demonstration

Open the Grafana dashboard in one browser tab and the Prometheus Alerts page in a second tab:
* **Grafana:** `http://localhost:3001/d/adg7ldb/workhub-enterprise-observability-and-operations`
  *(Set dashboard refresh interval in top-right to `5s`)*.
* **Prometheus:** `http://localhost:9090/alerts`

Execute the interactive simulation tool:
```bash
./scripts/simulate_traffic.sh
```

---

### Demo 2.1: Live Traffic Simulation (Scenario 1)
* **Action:** Choose option `1` (or run `./scripts/simulate_traffic.sh normal 30`).
* **What to Observe in Grafana:**
  - **API Requests per Second (RPS):** Gauge rises from 0 to 15–25 RPS with green throughput curves.
  - **HTTP Request Duration (P50/P95/P99):** Latency percentiles calculate dynamically in milliseconds.
  - **Database Connection Pool:** Active and idle connections fluctuate as queries execute.
  - **Cache Operations:** Cache hits and misses register on the Redis operations panel.

---

### Demo 2.2: Rate Limit 429 Storm & Alert Demonstration (Scenario 2)
* **Action:** Choose option `2` (or run `./scripts/simulate_traffic.sh ratelimit`).
* **Terminal Output:**
  ```text
  Dispatching 15 rapid authentication requests in sub-seconds...
    Request #1:  HTTP 200 OK (Allowed)
    ...
    Request #5:  HTTP 200 OK (Allowed)
    Request #6:  HTTP 429 Too Many Requests (Blocked by Rate Limiter!)
    ...
    Request #15: HTTP 429 Too Many Requests (Blocked by Rate Limiter!)
  ```
* **What to Observe:**
  1. **Grafana Stat Card:** The **`Rate Limit 429 Drops`** card in Row 1 immediately jumps to bright **Orange/Red** showing the drop count.
  2. **Prometheus Alerts (`:9090/alerts`):** The alert **`HighRateLimitRejections`** transitions immediately to **`FIRING 🔥`**.

---

### Demo 2.3: Circuit Breaker Full Lifecycle & Half-Open State (Scenario 3)
* **Action:** Choose option `3` (or run `./scripts/simulate_traffic.sh breaker`).
* **Execution Progression:**
  1. **Phase 1 (Baseline):** Circuit is in `CLOSED (HEALTHY 🟢 - 0)`.
  2. **Phase 2 (Trip to Open):** Script runs `docker stop workhub_redis` and sends 4 requests.
     - Circuit trips to **`OPEN (TRIPPED 🔴 - 2)`**.
     - Prometheus alert **`CacheCircuitBreakerOpen`** transitions immediately to **`FIRING 🔥`**.
  3. **Phase 3 (Recovery):** Script runs `docker start workhub_redis` and waits for the 10-second cooldown timer.
  4. **Phase 4 (Half-Open Probing):** Script sends 1 probe request.
     - Circuit breaker enters **`HALF-OPEN (PROBING 🟡 - 1)`**.
     - **Grafana Stat Card & State Timeline:** Display the yellow `HALF-OPEN` status prominently.
  5. **Phase 5 (Full Recovery):** Script sends remaining probe requests.
     - Circuit heals to **`CLOSED (HEALTHY 🟢 - 0)`**.
     - Prometheus alert clears and resolves.

---

## 4. Part 3: Automated Disaster Recovery Verification

Execute the automated backup and restore drill to verify RPO and RTO compliance:

```bash
./scripts/backup_restore_drill.sh
```
* **Validation Check:**
  - Automated `pg_dump` takes snapshot.
  - Catastrophic failure simulated (`DROP DATABASE workhub_dev`).
  - Database restored and verified via `md5` checksum calculation.
  - Verification completes with **RTO measured $\le 5$ seconds** and **RPO = 0 seconds**.
