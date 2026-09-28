# Day 7 Evidence — Repositories, transactions, and consistency

## Automated checks

```text
go test ./...                         PASS
go vet ./...                          PASS
go build -o bin/server ./cmd/server  PASS
```

Integration tests were executed against PostgreSQL 16.15 with `WORKHUB_TEST_DATABASE_URL`. The following tests passed:

| Test | Result |
|---|---|
| `CreateTaskWithAuditCommitsAtomically` | PASS — one task and one audit row committed. |
| `CreateTaskWithAuditRollsBackOnAuditFailure` | PASS — invalid audit event caused constraint error and left zero task rows. |
| `MapsNotFoundAndTimeout` | PASS — `ErrNotFound` and context deadline/cancellation are preserved. |
| `MigrationFilesExist` | PASS |

## Runtime demonstration

The server was started with:

```bash
DATABASE_URL='postgresql:///workhub_day7?host=/var/run/postgresql&user=ubuntu' PORT=18081 ./bin/server
```

The REST flow succeeded against PostgreSQL:

```text
POST /api/v1/projects                 -> project id 2
POST /api/v1/projects/2/tasks         -> task id 7
GET  /api/v1/projects/2/tasks?page=1&limit=1 -> total 1, one returned item
SELECT count(*) FROM audit_events WHERE task_id = 7 -> 1
```

This demonstrates that the API no longer depends on the in-memory repository when `DATABASE_URL` is configured, and that task creation writes its audit event atomically.
