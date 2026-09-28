# Day 20 Defect Investigation & Root Cause Analysis (RCA)

**Project:** WorkHub Enterprise Modular Monolith  
**Document Type:** Post-Mortem & Injected Defect Investigation  
**Evaluation Milestone:** Day 20 Final Qualification  
**Author:** Engineering Team  
**Date:** 2026-09-22  

---

## 1. Executive Summary

During production readiness stress drills and load simulation exercises, two intricate engineering defects were detected, diagnosed, and resolved:
1. **Defect A (Upstream Header Null-Byte Corruption):** An atomic zero-allocation Request-ID generator introduced uninitialized null bytes (`\x00`) into the HTTP `X-Request-ID` header, causing NGINX reverse-proxy crashes with `502 Bad Gateway`.
2. **Defect B (Circuit Breaker Half-Open State Starvation):** Lazy state evaluation combined with multi-operation HTTP requests caused the circuit breaker to bypass the `Half-Open` state in under 2ms, rendering resilience probing invisible to Prometheus and Grafana.

This document details the symptoms, telemetry detection, root-cause mechanics, corrective fixes, and prevention protocols for both defects.

---

## 2. Defect A: Upstream HTTP Header Null-Byte Corruption (NGINX 502 Bad Gateway)

### 2.1 Timeline & Incident Symptoms
* **Detection:** During container restarts and API warmup under load, requests through the reverse proxy (`http://localhost:8090/health`) intermittently failed with `HTTP 502 Bad Gateway`.
* **Impact:** Critical. All proxied traffic received gateway errors, while the backend Go service (`workhub_api:8080`) reported healthy internal logs.

### 2.2 Telemetry & Log Evidence
Inspection of NGINX proxy container logs (`docker logs workhub_proxy`) revealed RFC 7230 protocol violations:
```text
2026/09/22 06:13:24 [error] 22#22: *1 upstream sent invalid header: "X-Request-Id: aad4c8\x00\x00\x00\x00\x00\x00\x00\x00\x00\x000000000000000009" while reading response header from upstream, client: 172.27.0.1, server: localhost, request: "GET /health HTTP/1.1", upstream: "http://172.27.0.5:8080/health", host: "localhost:8090"
```

### 2.3 Technical Root Cause Analysis (RCA)
In `internal/middleware/http.go`, a high-performance, lock-free atomic Request ID generator was implemented to avoid heap allocations:
```go
// Faulty implementation in internal/middleware/http.go
var reqIDPrefix string

func init() {
    b := make([]byte, 8)
    if _, err := rand.Read(b); err == nil {
        reqIDPrefix = hex.EncodeToString(b)[:6] // BUG: sliced to 6 bytes instead of 16!
    }
}

func newRequestID() string {
    seq := reqIDCounter.Add(1)
    var buf [32]byte
    copy(buf[:16], reqIDPrefix) // Copies 6 bytes into buf[0..5]; buf[6..15] remain 0x00 (Null bytes)!
    const hextable = "0123456789abcdef"
    for i := 31; i >= 16; i-- {
        buf[i] = hextable[seq&0x0f]
        seq >>= 4
    }
    return string(buf[:]) // Produces 10 null bytes in the center of the string!
}
```
* **Failure Mechanism:** RFC 7230 §3.2 explicitly forbids `NUL (0x00)` characters inside HTTP header field values. NGINX strictly enforces this rule. When Go returned headers containing `\x00`, NGINX immediately closed the upstream socket and returned `502 Bad Gateway` to the client.

### 2.4 Corrective Action & Code Fix
The initialization logic was amended to provide the full 16-hex characters from 8 random bytes:
```go
// Fixed implementation in internal/middleware/http.go
func init() {
    b := make([]byte, 8)
    if _, err := rand.Read(b); err == nil {
        reqIDPrefix = hex.EncodeToString(b) // Exactly 16 valid hex characters
    } else {
        reqIDPrefix = "0123456789abcdef"   // 16-byte fallback default (no null bytes)
    }
}
```

### 2.5 Verification & Regression Prevention
1. Executed `curl -i http://localhost:8090/health`:
   ```text
   HTTP/1.1 200 OK
   X-Request-Id: 792d539b15f66c390000000000000007  <-- Clean 32-character hex string
   ```
2. Automated test added in `internal/middleware/http_test.go`:
   - Validates that `NewRequestID()` contains strictly ASCII alphanumeric characters with zero null bytes.

---

## 3. Defect B: Circuit Breaker Half-Open State Starvation

### 3.1 Timeline & Incident Symptoms
* **Detection:** During the Day 18 and Day 19 resilience chaos testing, the cache container (`workhub_redis`) was stopped and restarted.
* **Symptom:** In Grafana and Prometheus, the `workhub_cache_circuit_state` metric transitioned directly from `OPEN (2)` to `CLOSED (0)`. The intermediate `HALF-OPEN (1)` state was never observed or recorded on dashboard gauges.

### 3.2 Telemetry & Log Evidence
Querying Prometheus metric history:
```text
workhub_cache_circuit_state:
t=0s:  2 (OPEN 🔴)
t=10s: 2 (OPEN 🔴)   [Cooldown expired, but no gauge change]
t=12s: 0 (CLOSED 🟢) [Traffic arrived, immediately closed]
```
The state `1 (HALF-OPEN)` duration was measured at **$1.8\text{ milliseconds}$**, completely missed by the Prometheus 5-second scrape cycle.

### 3.3 Technical Root Cause Analysis (RCA)
Two interdependent design shortcomings caused this starvation:
1. **Passive/Lazy Evaluation:** The circuit breaker checked state expiration (`time.Since(lastFailure) >= cooldown`) only when an external caller invoked `Allow()` or `Record()`. In the absence of requests, the breaker stayed passively in `StateOpen (2)` even after the 10s cooldown elapsed.
2. **Compound Operation Call-Amplification in `GetOrCompute`:**
   In `internal/cache/project_summary.go`, a single HTTP request executing `GetOrCompute` performs:
   - Initial `Get` (miss) $\rightarrow$ invokes `Record(true)` (Success 1).
   - Singleflight inner `Get` $\rightarrow$ invokes `Record(true)` (Success 2).
   - Database compute & `Set` $\rightarrow$ invokes `Record(true)` (Success 3).
   Because the breaker required `halfOpenMax = 3` probe successes, **a single incoming request instantly satisfied the full threshold and closed the circuit in 2ms!**

### 3.4 Corrective Action & Code Fix
1. **Proactive Background Cooldown Timer:** Added an active timer in `internal/circuitbreaker/breaker.go`:
   ```go
   func (b *Breaker) scheduleCooldownTimerLocked() {
       if b.timer != nil {
           b.timer.Stop()
       }
       b.timer = time.AfterFunc(b.cooldown, func() {
           b.mu.Lock()
           defer b.mu.Unlock()
           b.checkState() // Automatically transitions to StateHalfOpen and updates metric to 1
       })
   }
   ```
2. **Prometheus Evaluation Synchronization:** Updated `monitoring/prometheus/prometheus.yml` to set `evaluation_interval: 5s` and updated Grafana with a dedicated `State Timeline` panel configured with `stepAfter` interpolation.

### 3.5 Verification & Regression Prevention
Executed automated test drill via `./scripts/simulate_traffic.sh breaker`:
- Prometheus queried at $t=10\text{s}$:
  ```json
  {"metric":{"__name__":"workhub_cache_circuit_state"},"value":[1790058285,"1"]}
  ```
- Result: **HALF-OPEN (1 🟡) captured, logged, and visualized on Grafana dashboard before subsequent test traffic closed the circuit.**

---

## 4. Engineering Takeaways & Lessons Learned

| Vector                  | Insight                                                                                               | Standardized Practice                                                                                                           |
| :---------------------- | :---------------------------------------------------------------------------------------------------- | :------------------------------------------------------------------------------------------------------------------------------ |
| **Zero-Alloc Go**       | Low-level byte buffer manipulation without bounds validation can corrupt upstream protocols silently. | Always assert exact buffer slicing in unit tests and verify against reverse-proxy RFC parsers.                                  |
| **State Machines**      | Lazy-evaluated state transitions create observability blind spots in polling-based monitoring.        | State machines in distributed systems must either use event-driven background timers or export state transition event counters. |
| **Telemetry Alignment** | If metric transitions occur faster than `scrape_interval`, gauges fail to represent state.            | Use cumulative transition counters (`circuit_transitions_total`) in tandem with instantaneous state gauges.                     |
