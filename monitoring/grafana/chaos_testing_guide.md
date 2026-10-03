# Chaos Engineering - WorkHub

This document contains actionable scenarios to test system resilience and observe Grafana dashboard behavior during failures and high traffic loads.

## 1. Spike Traffic 🚀
Floods the server with rapid, lightweight requests to monitor latency responsiveness and overall throughput increase on the dashboard.

**Target in Grafana:**
- Spikes **`API Request Rate`**.
- Spikes **`DB Queries Rate`**.

**Command:**
```bash
for i in {1..500}; do curl -s -X GET http://localhost:8080/health > /dev/null & done; wait
```

---

## 2. Error Storm (4xx and 5xx) 💥
Sends heavy bursts of requests to non-existent or unauthorized routes to trigger error metrics.

**Target in Grafana:**
- Sharp rise in **`API Error Rate`** with yellow (4xx) and red (5xx) thresholds.

**Command:**
```bash
for i in {1..100}; do 
  curl -s -X GET http://localhost:8080/api/v1/projects/999999 > /dev/null & 
  curl -s -X GET http://localhost:8080/api/v1/fake-url > /dev/null & 
done; wait
```

---

## 3. Latency Spike Simulation 🐢
Sends requests requiring heavy CPU computation (e.g. concurrent login attempts involving bcrypt password hashing).

**Target in Grafana:**
- Spike in **`API Latency P95`** (approaching 1s or more per request).

**Command:**
```bash
for i in {1..30}; do 
  curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"owner@workhub.dev","password":"Password1234!"}' > /dev/null & 
done; wait
```

---

## 4. Cache Circuit Breaker Drill 🔌
Simulates sudden Redis cache failure to verify the system detects failure and fails open gracefully to the database.

**Target in Grafana:**
- **`Cache Circuit Breaker State`** transitions from `0` (Green / CLOSED) to `2` (Red / OPEN), indicating circuit tripped and bypassing Redis.

**Command (to stop Redis):**
```bash
docker stop workhub_redis
```
*(Send cache requests to trip the circuit threshold)*.

**Command (to recover Redis):**
```bash
docker start workhub_redis
```
*(Circuit transitions to `1` HALF-OPEN for probing, then resets to `0` CLOSED once healthy).*

---

## 5. Load Shedding & Bulkhead Protection 🛡️
Sends massive concurrency bursts exceeding maximum allowed in-flight requests, forcing graceful rejection.

**Target in Grafana:**
- **`Load Shedding / Bulkhead Rejections`** panel increments to record shedding events.

**Command:**
```bash
for i in {1..2000}; do curl -s http://localhost:8080/health > /dev/null & done; wait
```
