# WorkHub Production Incident Response & Troubleshooting Runbook

**Document Type:** Incident Response Runbook & Production Triage Manual  
**Scope:** Runtime Anomaly Investigation, Failure Remediation, and Service Restoration  
**Audience:** Site Reliability Engineers (SRE), On-Call Engineers, and Operations Teams  

---

## 1. Common Incident Scenarios & Remediation Protocols

### Scenario 1: Readiness Probe Returns `503 Service Unavailable` on `/ready`

* **Symptoms:** The `/ready` health check fails; container orchestrator marks the API pod as `unhealthy` and withdraws it from ingress rotation.
* **Root Causes:** Loss of network connectivity to PostgreSQL, database pool exhaustion, or unready Redis cache.
* **Diagnostic & Remediation Steps:**
  ```bash
  # 1. Inspect API logs to isolate the exact failing dependency
  docker compose logs api --tail 30

  # 2. Inspect PostgreSQL container state and logs
  docker compose ps postgres
  docker compose logs postgres --tail 20

  # 3. Test direct database connectivity from the CLI
  docker compose exec postgres psql -U workhub -d workhub_dev -c "SELECT 1;"

  # 4. If the database container is stopped or unhealthy, restart dependencies
  docker compose restart postgres api
  ```

---

### Scenario 2: High Frequency of `429 Too Many Requests` Client Errors

* **Symptoms:** Clients receive HTTP `429` with a `Retry-After` header.
* **Root Causes:** Client exceeding rate quotas (5 requests/10s on auth endpoints, 10 projects/min), Layer 7 DoS attack, or Nginx failing to pass client IP via `X-Forwarded-For`.
* **Diagnostic & Remediation Steps:**
  ```bash
  # 1. Identify offending IP addresses in API logs
  docker compose logs api | grep "rate limit" | tail -n 20

  # 2. Verify that the ingress proxy properly forwards the client IP
  curl -i -H "X-Forwarded-For: 203.0.113.195" http://localhost:8090/health

  # 3. If legitimate traffic requires higher throughput, adjust rate limit parameters in internal/handler/api.go
  ```

---

### Scenario 3: Requests Rejected with `401 Unauthorized` or `Replay Attack` Warning

* **Symptoms:** Users are unexpectedly logged out. Logs indicate: `token replay attack detected! revoking all user sessions`.
* **Root Causes:** A previously used refresh token was submitted again, or client concurrency submitted duplicate token exchange requests simultaneously over high-latency networks.
* **Diagnostic & Remediation Steps:**
  * **Expected Security Behavior:** This is intentional defense-in-depth behavior to protect user accounts against credential theft.
  * The system immediately revokes all descendant tokens in the family.
  * **User Action:** The client must re-authenticate with primary credentials (`POST /api/v1/auth/login`) to generate a new token lineage.

---

### Scenario 4: Background Worker Fails to Process Events or Notifications

* **Symptoms:** Events publish successfully from the API, but notification webhooks are not dispatched, or outbox records remain in `pending` state.
* **Root Causes:** Dedicated `workhub_broker` disconnected, or `workhub_worker` container crashed/starved.
* **Diagnostic & Remediation Steps:**
  ```bash
  # 1. Inspect background worker container status and logs
  docker compose logs worker --tail 50

  # 2. Query worker internal health probe
  docker compose exec worker wget -qO- http://localhost:9091/healthz

  # 3. Inspect Redis broker health
  docker compose logs broker --tail 20

  # 4. Restart worker and broker if unresponsive
  docker compose restart broker worker
  ```

---

## 2. Cluster Diagnostic & Verification Commands

```bash
# Follow live aggregated logs across all services
docker compose logs --tail 20 --follow

# Review CPU and memory allocation to identify potential OOM risks
docker stats --no-stream

# Audit open PostgreSQL connection states
docker compose exec postgres psql -U workhub -d workhub_dev -c \
  "SELECT count(*), state FROM pg_stat_activity GROUP BY state;"
```
