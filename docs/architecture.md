# WorkHub Scaled Architecture

This document describes the scaled architecture of WorkHub, built to handle high concurrency, ensure reliability, and provide extensive observability.

## Architecture Diagram

```mermaid
graph TD
    %% External Entities
    Client[Client / Browser]
    
    %% Ingress & Edge
    subedge[Edge Layer]
    LB[Load Balancer / Ingress]
    end

    %% Application Layer
    subapp[Application Layer (workhub-service)]
    API[HTTP API Handler]
    MW[Middleware Chain\n- Auth\n- LoadShedding\n- RequestID]
    Service[Service Layer]
    Bulkhead[Bulkhead Isolation]
    end

    %% Background Workers
    subworker[Background Workers]
    WorkerPool[Worker Pool\n(Goroutines)]
    EventBroker[In-Memory Event Broker]
    end

    %% Data Layer
    subdata[Data Layer]
    DB[(PostgreSQL\nPrimary)]
    Redis[(Redis\nCache & Session)]
    end

    %% Observability Layer
    subobs[Observability Layer]
    Prometheus[Prometheus\n(Metrics)]
    Grafana[Grafana\n(Dashboards)]
    OTelCollector[OTel Collector\n(Tracing)]
    end

    %% Flow
    Client -->|HTTPS| LB
    LB --> API
    API --> MW
    MW --> Service
    
    Service -->|Uses| Bulkhead
    Bulkhead -->|Executes DB Queries| DB
    
    Service -->|Circuit Breaker| Redis
    
    %% Async Flow
    Service -.->|Publishes Domain Events| EventBroker
    EventBroker -.->|Consumes| WorkerPool
    WorkerPool -->|Dead Letter / Retry| Redis
    
    %% Telemetry Flow
    API -.->|Exposes /metrics| Prometheus
    Prometheus -.-> Grafana
    Service -.->|Exports Spans| OTelCollector
```

## Key Architectural Patterns Used

1. **Load Shedding:** Middleware protects the service from being overwhelmed by dropping requests quickly (503 Service Unavailable) when max concurrent requests are reached.
2. **Bulkheads:** Database connection pools are protected by semaphores in the code to ensure that a slow query (e.g., in the `tasks` endpoint) doesn't consume all resources and block `projects` or `users` endpoints.
3. **Circuit Breakers:** Redis operations (e.g., Project Summaries) are protected by a 3-state circuit breaker. If Redis is down, the system gracefully falls back to direct DB queries (or degrades safely) rather than hanging.
4. **Idempotency:** Mutations implement Idempotency Keys stored in Redis to safely retry operations without duplicating data.
5. **Outbox Pattern / Event Broker:** Domain events are committed transactionally to the database (`outbox_events`) to prevent data inconsistency, then published to the worker pool.
6. **Worker Pool with Dead-Letter Queue:** Background tasks are limited to a fixed concurrency pool with automatic retries and dead-lettering to Redis upon maximum failures.
7. **Comprehensive Observability:** OpenTelemetry distributed tracing and Prometheus RED metrics are deeply integrated into the service layers and database wrappers.
