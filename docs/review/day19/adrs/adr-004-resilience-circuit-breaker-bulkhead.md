# ADR-004: Multi-Layer Resilience Architecture & Graceful Degradation

* **Status:** Accepted
* **Date:** 2026-09-19
* **Decision Makers:** SRE, Reliability Engineering, Platform Architecture

---

## Context
In distributed microservices and modular monolith architectures, latency spikes or total failure of auxiliary services (such as distributed caches or message brokers) risk exhausting application connection pools and triggering catastrophic cascading cluster failure.

## Decision
Implement a defense-in-depth resilience pattern across four complementary runtime layers:
1. **Circuit Breaker:** Automatically trips open when Redis encounters 5 consecutive errors, immediately activating a **Fail-Open** fallback directly to PostgreSQL without stalling incoming requests.
2. **Bulkhead Isolation:** Restricts heavy, unbounded analytical queries behind separate concurrency throttles to prevent worker thread starvation.
3. **Adaptive Load Shedding:** Evaluates in-flight request capacity (`MaxInFlight`). If concurrent traffic breaches safety thresholds, excess requests are rejected with HTTP `503 Service Unavailable` and a `Retry-After: 5` header.
4. **Graceful Shutdown:** Configures a 30-second termination grace period allowing active requests and background jobs to complete upon receiving `SIGTERM` or `SIGINT`.

## Consequences & Trade-offs
* **Positive:** Prevents out-of-memory (OOM) crashes and cascading dependency starvation; preserves 100% core uptime even during severe cache disruptions.
* **Negative:** Extreme peak traffic beyond system limits experiences controlled 503 load shedding responses to protect active operations.

## Verification Commands
```bash
go test -v -race ./tests/performance/ratelimit_load_test.go -run="TestGracefulDegradation"
```
