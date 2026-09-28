# ADR-001: Separation of Query Cache from Distributed Event Broker

* **Status:** Accepted
* **Date:** 2026-09-20
* **Decision Makers:** Architecture Core Team, SRE

---

## Context
During early development phases, a single shared Redis instance was utilized for both low-latency query caching (Project Summaries, Idempotency keys) and asynchronous event streaming (Pub/Sub and background worker queues). In production environments, high query write volume and memory pressure trigger key eviction policies (`allkeys-lru`), risking accidental eviction or starvation of critical in-flight domain events destined for background worker execution.

## Decision
Decouple the event broker from the query cache into two isolated Redis services operating across independent network configurations:
1. **`workhub_redis` (Port 6379):** Dedicated strictly to query caching and idempotency locks with an eviction policy of `volatile-lru`.
2. **`workhub_broker` (Port 6379, isolated internal network):** Dedicated strictly to Redis Pub/Sub and transactional Outbox delivery with eviction disabled (`noeviction`).

## Consequences & Trade-offs
* **Positive:** Complete blast radius isolation; cache saturation cannot corrupt or drop background worker events; memory quotas and alerting thresholds are governed independently.
* **Negative:** Modest infrastructure footprint increase; requires maintaining two distinct connection pool configurations in application configuration.

## Verification Commands
```bash
docker compose ps | grep -E "broker|redis"
go test -v -race ./internal/cache/... ./internal/messaging/...
```
