# ADR-001: Separation of Redis Cache and Redis Message Broker

## Status
Accepted (Production-Ready Architecture)

## Context
WorkHub utilizes in-memory and distributed caching for project summaries and idempotency keys, as well as a messaging mechanism for dispatching domain events (such as `task.created`) to background notification workers.

In development environments, developers often run a single Redis instance to serve all functions. However, when transitioning to containerized production operations (Day 16), running cache operations and asynchronous message broker workloads on the same instance introduces architectural risks.

## Decision
We decouple **Redis Cache** (`redis`) and **Redis Message Broker** (`broker`) into two dedicated, independently configured container services in Docker Compose and production topologies:
1. **`workhub_redis`**: Dedicated caching and idempotency storage.
   - Configured with persistence (`--save 60 1`) and memory eviction policies appropriate for volatile query caches (e.g. `allkeys-lru`).
2. **`workhub_broker`**: Dedicated event bus and asynchronous pub/sub messaging broker.
   - Configured with persistence disabled (`--save "" --appendonly no`) for low-latency in-memory delivery, zero disk I/O contention, and dedicated buffers for real-time subscribers.

## Consequences

### Positive (Benefits)
1. **Fault Isolation (Blast Radius Containment)**:
   - If a heavy analytical query or high-volume cache write fills the cache memory or causes eviction, it will never disrupt, evict, or drop domain events in the messaging channel.
   - If pub/sub throughput spikes, it will not degrade API query response times or exhaust Redis connections needed for request authentication and idempotency validation.
2. **Independent Tuning and Resource Limits**:
   - Cache service can be provisioned with larger memory allocation and snapshot persistence.
   - Broker service can be tuned strictly for socket throughput, low latency, and zero fsync stalls.
3. **Seamless Migration Path**:
   - Isolating the message broker via an explicit `BROKER_URL` allows frictionless migration to a dedicated enterprise broker (e.g., RabbitMQ, Apache Kafka, or AWS SQS) in future iterations without refactoring cache logic.

### Negative (Trade-offs)
- Slightly higher baseline memory footprint (two Redis container instances instead of one: ~15MB each on Alpine).
- Additional service definition to orchestrate in Docker Compose.
