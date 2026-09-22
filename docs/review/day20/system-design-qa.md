# Day 20 System Design & Scalability Defense (Q&A)

**Project:** WorkHub Enterprise Modular Monolith  
**Evaluation Phase:** Day 20 Final Qualification (Weekly Task 4)  
**Author:** Lead Distributed Systems Engineer  
**Date:** 2026-09-22  

---

## Question 1: How would you scale WorkHub to handle 10,000+ Requests Per Second (100x Scale)?

### Architecture Evolution:
Scaling from baseline (~100–500 RPS) to 10,000+ RPS requires eliminating single-node chokepoints across the application, database, and caching tiers:

```text
               +-------------------------------------------------------+
               |         Cloud Layer 7 Load Balancer (AWS ALB / Cloudflare)    |
               +---------------------------+---------------------------+
                                           |
             +-----------------------------+-----------------------------+
             |                             |                             |
             v                             v                             v
   +-------------------+         +-------------------+         +-------------------+
   |  WorkHub API Pod  |         |  WorkHub API Pod  |         |  WorkHub API Pod  |
   |   (Stateless Go)  |         |   (Stateless Go)  |         |   (Stateless Go)  |
   +---------+---------+         +---------+---------+         +---------+---------+
             |                             |                             |
             +-----------------------------+-----------------------------+
                                           |
             +-----------------------------+-----------------------------+
             |                                                           |
             v                                                           v
   +--------------------+                                      +--------------------+
   |   Redis Cluster    |                                      |     PgBouncer      |
   | (6-node Sharded)   |                                      | (Connection Pooler)|
   +--------------------+                                      +---------+----------+
                                                                         |
                                                   +---------------------+---------------------+
                                                   |                                           |
                                                   v (Writes)                                  v (Reads)
                                         +-------------------+                       +-------------------+
                                         | PostgreSQL Primary|---(Async Replication)-|PostgreSQL Replica |
                                         |    (Write Head)   |                       |    (Read Pool)    |
                                         +-------------------+                       +-------------------+
```

1. **Stateless Compute Layer (Go API):**
   - WorkHub handles authentication statelessly via JWTs; instances do not share state.
   - Deploy as a Kubernetes Deployment with a Horizontal Pod Autoscaler (HPA) targeting 65% CPU utilization and P95 latency $< 100\text{ms}$.
2. **Database Read/Write Splitting & Connection Pooling:**
   - PostgreSQL Primary handles writes; 2–4 Read Replicas handle `GET /api/v1/projects` and `GET /api/v1/tasks`.
   - Implement **PgBouncer** in `transaction pooling mode` to maintain 10,000 client frontend connections multiplexed across 50 dedicated backend PostgreSQL connections.
   - **Replication Lag Mitigation:** When a user creates a task, return an `LSN (Log Sequence Number)` cookie or route requests from that user to Primary for 2 seconds to guarantee Read-Your-Own-Writes consistency.
3. **Distributed Caching (Redis Cluster):**
   - Transition from standalone Redis to a 6-node Redis Cluster with hash slots partitioned across shards, achieving 100,000+ ops/sec with sub-millisecond cache hits.

---

## Question 2: How does WorkHub prevent Cache Stampedes (Thundering Herd)?

### The Problem:
When a high-traffic cached key (such as `workhub:project-summary:3` requested 500 times/second) expires or is invalidated, all 500 incoming concurrent requests experience a cache miss simultaneously and attempt to recalculate the summary against PostgreSQL, crashing the database connection pool.

### The Solution Implemented in WorkHub:
WorkHub implements `golang.org/x/sync/singleflight`:
```go
func (c *ProjectSummaryCache) GetOrCompute(ctx context.Context, projectID string, computeFn ...) (domain.ProjectSummary, error) {
    if summary, ok := c.Get(ctx, projectID); ok {
        return summary, nil // 1. Cache hit
    }

    // 2. Singleflight deduplication
    val, err, _ := c.sf.Do(projectID, func() (any, error) {
        // Double-check cache inside mutex to handle race winner
        if summary, ok := c.Get(ctx, projectID); ok {
            return summary, nil
        }
        summary, computeErr := computeFn(ctx) // Executes exactly ONCE against PostgreSQL
        if computeErr != nil {
            return domain.ProjectSummary{}, computeErr
        }
        c.Set(ctx, summary) // Populates cache for future requests
        return summary, nil
    })
    return val.(domain.ProjectSummary), err
}
```
* **Outcome:** Under a burst of 1,000 concurrent cold requests, exactly **1 query** hits PostgreSQL. The other 999 goroutines block on the channel and share the single result in memory.

---

## Question 3: How would you design a Multi-Region Active-Active deployment?

### Architecture Strategy:
1. **Tenant Geo-Pinning (Data Sovereignty & Latency):**
   - Each enterprise tenant/organization is pinned to a primary home region (e.g., `EU-Central` or `US-East`).
   - Latency-based Anycast DNS routes client traffic to the nearest regional edge. If an EU user accesses a US project, the EU proxy forwards the request over the cloud backbone to the US region.
2. **Authentication Across Regions:**
   - Stateless JWT authentication means any regional API pod can authenticate access tokens using the identity provider's public keys (JWKS) cached locally.
3. **Asynchronous Cross-Region Replication:**
   - PostgreSQL logical replication replicates tenant metadata and audit trails across regions.
   - For global entities (e.g., user profiles), use **Conflict-Free Replicated Data Types (CRDTs)** or Last-Write-Wins (LWW) with Lamport timestamps to prevent split-brain conflicts.

---

## Question 4: How does WorkHub achieve Exactly-Once Processing semantics in Event-Driven Messaging?

### The Reality of Distributed Messaging:
In distributed networks, true network-level exactly-once delivery is physically impossible due to the Two Generals' Problem. Systems provide **at-least-once delivery** paired with **idempotent consumption** to achieve **effective exactly-once processing**.

### WorkHub's Implementation Pipeline:
1. **Transactional Outbox Pattern (Producer Guarantee):**
   - When a task is created, the task row and an `outbox` record (`event_id`, `aggregate_type`, `payload`) are committed in the **same PostgreSQL ACID transaction**:
     ```sql
     BEGIN;
     INSERT INTO tasks (...) VALUES (...);
     INSERT INTO outbox (event_id, topic, payload) VALUES ('evt_123', 'task.created', '{...}');
     COMMIT;
     ```
   - Eliminates Dual-Write Failure: Either both the task and outbox event exist, or neither does.
2. **Background Dispatcher:**
   - A dedicated worker polls the outbox table or uses Change-Data-Capture (Debezium) to publish events to RabbitMQ/Kafka with at-least-once delivery.
3. **Idempotent Consumer (Deduplication Gate):**
   - The notification consumer checks an idempotency ledger (`processed_events` table) inside a transaction:
     ```sql
     INSERT INTO processed_events (event_id, processed_at) 
     VALUES ('evt_123', NOW()) 
     ON CONFLICT (event_id) DO NOTHING;
     ```
   - If the insert returns `0 rows affected`, the message is an unhandled duplicate and is immediately acknowledged and discarded without re-sending notifications.

---

## Question 5: How do you mitigate "Noisy Neighbors" in a Multi-Tenant architecture?

### Multi-Tier Defense:
1. **Adaptive Tiered Rate Limiting:**
   - Users are tagged by tier (`Free`, `Admin`, `Enterprise Owner`). Rate limiters allocate token bucket capacity dynamically (`10 req/s` for Free vs `100 req/s` for Enterprise).
2. **Database Quota Isolation:**
   - WorkHub enforces strict DB-level quotas (`user_quotas` table) on total projects, members per project, and tasks per project checked atomically via row-level locks before resource creation.
3. **Concurrency Bulkheads:**
   - Compute-intensive queries (e.g., project export, report generation) execute within bounded goroutine worker pools. A single tenant flooding report requests saturates only their allocated semaphore without affecting other tenants' interactive API requests.
