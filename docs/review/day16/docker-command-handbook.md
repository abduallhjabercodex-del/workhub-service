# WorkHub Docker & Production Runtime Command Handbook

**Document Type:** Operations Reference Manual  
**Scope:** Container Building, Hardening, Orchestration, Health Monitoring, and Production Drills  
**Environment:** Local, Staging, and Production Docker Runtimes  

---

## Table of Contents

1. [Image Building & Size Optimization](#1-image-building--size-optimization)
2. [Security Verification & Least Privilege Hardening](#2-security-verification--least-privilege-hardening)
3. [Docker Compose Stack Orchestration](#3-docker-compose-stack-orchestration)
4. [Observability, Logs & Resource Metrics](#4-observability-logs--resource-metrics)
5. [The 4 Operational Production Drills](#5-the-4-operational-production-drills)
6. [Maintenance, System Hygiene & Pruning](#6-maintenance-system-hygiene--pruning)

---

## 1. Image Building & Size Optimization

```bash
# Build API image with multi-stage Dockerfile
docker build -f Dockerfile.api -t workhub-api:local .

# Build Background Worker image
docker build -f Dockerfile.worker -t workhub-worker:local .

# Force a clean rebuild bypassing the Docker build cache
docker build --no-cache -f Dockerfile.api -t workhub-api:local .

# Inspect generated images and verify size compliance (< 35 MB compressed binary gate)
docker images | grep workhub-

# Inspect layer composition and individual layer sizes
docker history workhub-api:local
```

---

## 2. Security Verification & Least Privilege Hardening

```bash
# Verify non-root execution (UID 10001:workhub)
docker run --rm --entrypoint id workhub-api:local
docker run --rm --entrypoint id workhub-worker:local

# Verify read-only root filesystem enforcement (read_only: true)
docker compose exec api touch /root/test.txt 2>&1 || echo "Filesystem is Read-Only (SECURE)"

# Verify writable in-memory temporary scratchpad (tmpfs: /tmp)
docker compose exec api touch /tmp/test.txt && docker compose exec api rm /tmp/test.txt

# Inspect security options and dropped Linux capabilities
docker inspect workhub_api --format '{{json .HostConfig.SecurityOpt}}'
docker inspect workhub_api --format '{{json .HostConfig.CapDrop}}'
```

---

## 3. Docker Compose Stack Orchestration

```bash
# Validate compose syntax and environment variable substitution
docker compose config --quiet

# Launch entire 6-service stack in detached daemon mode
docker compose up -d

# Rebuild images and launch containers simultaneously
docker compose up -d --build

# Check running status and health check state of all services
docker compose ps

# Pause services temporarily without removing containers
docker compose stop

# Restart specific application containers
docker compose restart api worker

# Tear down containers and networks while preserving database volumes
docker compose down

# Full reset: tear down containers, networks, and persistent volumes (Destructive!)
docker compose down -v
```

---

## 4. Observability, Logs & Resource Metrics

```bash
# Real-time CPU and Memory utilization across all active containers
docker stats --no-stream

# Follow logs from all services concurrently
docker compose logs -f

# Tail specific container logs (last 50 lines)
docker compose logs api --tail 50 -f
docker compose logs worker --tail 50 -f
docker compose logs postgres --tail 50

# Inspect isolated container bridge network and allocated internal IPs
docker network inspect workhub-dev_workhub-net

# Inspect named persistent data volumes
docker volume ls | grep workhub
```

---

## 5. The 4 Operational Production Drills

### Drill 1: Graceful Shutdown Drill
```bash
# Issue SIGTERM to API and Worker services
docker compose stop api worker

# Verify signal capture and clean in-flight task draining in logs
docker compose logs api --tail 20
docker compose logs worker --tail 20

# Restart services for subsequent operations
docker compose start api worker
```

---

### Drill 2: Data Persistence Across Full Teardown Drill
```bash
# 1. Insert a test project record directly into PostgreSQL
docker compose exec postgres psql -U workhub -d workhub_dev -c \
  "INSERT INTO projects (name, owner_user_id) VALUES ('Day 16 Persistence Drill Project', 1) RETURNING id, name, created_at;"

# 2. Completely destroy all containers and networks
docker compose down

# 3. Boot the stack fresh from cold storage
docker compose up -d

# 4. Query the inserted project record to prove persistent volume survival
docker compose exec postgres psql -U workhub -d workhub_dev -c \
  "SELECT id, name, created_at FROM projects WHERE name='Day 16 Persistence Drill Project';"
```

---

### Drill 3: Database Backup & Point-in-Time Recovery Drill
```bash
# Extract full database dump from PostgreSQL container
docker compose exec -T postgres pg_dump -U workhub workhub_dev > docs/review/day16/evidence/backup.sql

# Validate SQL dump header and table schemas
head -n 15 docs/review/day16/evidence/backup.sql

# Restore SQL backup to active database (Disaster Recovery Simulation)
docker compose exec -T postgres psql -U workhub workhub_dev < docs/review/day16/evidence/backup.sql
```

---

### Drill 4: Dependency Failure & Self-Healing Drill
```bash
# 1. Terminate database container to simulate sudden infrastructure outage
docker compose stop postgres

# 2. Probe readiness endpoint (must return HTTP 503 Service Unavailable)
curl -i http://localhost:8090/ready

# 3. Recover database service
docker compose start postgres

# 4. Wait for database healthcheck, then probe readiness (auto-recovers to HTTP 200 OK)
sleep 8
curl -i http://localhost:8090/ready
```

---

## 6. Maintenance, System Hygiene & Pruning

```bash
# View disk usage consumed by Docker artifacts
docker system df

# Prune stopped containers and unused networks
docker container prune -f
docker network prune -f

# Prune untagged dangling build images
docker image prune -f

# Comprehensive cleanup (removes all unused containers, networks, and images)
docker system prune -a --volumes -f
```
