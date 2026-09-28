# ADR-003: Zero-Downtime Database Migration Strategy (Expand & Contract Pattern)

* **Status:** Accepted
* **Date:** 2026-09-20
* **Decision Makers:** Database Engineering, DevOps, SRE

---

## Context
Executing destructive in-place schema changes during rolling or blue-green container deployments inevitably causes breaking errors when older and newer application replicas run concurrently against the shared database cluster.

## Decision
Prohibit single-step destructive schema modifications and mandate the **Expand and Contract Pattern** across four discrete engineering phases:
1. **Phase 1 (Expand):** Add new columns or tables safely without removing or altering legacy structures (`ADD COLUMN nullable`).
2. **Phase 2 (Dual-Write):** Deploy application logic that writes to both legacy and new structures while maintaining read paths on legacy structures. Backfill existing historical data.
3. **Phase 3 (Read Switch):** Deploy application logic that switches reads and writes exclusively to the new schema structures.
4. **Phase 4 (Contract):** After verifying platform stability across multiple release cycles, execute a cleanup migration to drop obsolete legacy columns.

## Consequences & Trade-offs
* **Positive:** True zero-downtime continuous deployment; non-breaking backward compatibility; rollback capability remains intact at every step.
* **Negative:** Requires multiple coordinated deployment cycles to complete a single structural schema alteration.

## Verification Commands
```bash
./db/migrations/run_migrations.sh
docker compose exec postgres psql -U workhub -d workhub_dev -c "SELECT * FROM schema_migrations;"
```
