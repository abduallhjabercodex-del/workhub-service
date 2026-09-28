# Evidence — Days 2–3

## Build and tests

Commands executed successfully:

```text
gofmt -w cmd/server internal
go test ./...
go build -o bin/server ./cmd/server
```

The handler integration tests cover project creation, task creation, task update, and validation error envelopes.

## Health response

```http
HTTP/1.1 200 OK
Content-Type: application/json
X-Request-Id: df1225768e78d3c9

{"status":"ok","time":"2026-09-02T05:58:35Z"}
```

## REST flow

The verified flow was:

1. `POST /api/v1/projects` returned `201 Created`.
2. `POST /api/v1/projects/{projectId}/tasks` returned `201 Created`.
3. `GET /api/v1/projects/{projectId}/tasks?status=todo&page=1&limit=20` returned the task and pagination metadata.
4. `PATCH /api/v1/tasks/{taskId}` changed status from `todo` to `done`.

## Architecture evidence

The dependency direction is `handler → service → repository`, with `domain` independent from transport. Middleware adds request IDs, structured logs, panic recovery, and request timeouts. The repository is an in-memory implementation injected into the service at startup.
