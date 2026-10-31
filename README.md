# Webhook Delivery Platform

A backend service that accepts platform events (e.g. `user.created`, `payment.succeeded`) and delivers them as signed HTTP POSTs to customer-registered webhook endpoints. Delivery is asynchronous and every attempt is recorded. Failures are retried with exponential backoff.

## Tech stack

| Concern        | Choice                                   |
|----------------|------------------------------------------|
| Language       | Go 1.26                                  |
| HTTP router    | [chi v5](https://github.com/go-chi/chi)  |
| Database       | PostgreSQL 16 via [pgx v5](https://github.com/jackc/pgx) |
| Queue          | Redis 7 sorted set (delay queue) via [go-redis v9](https://github.com/redis/go-redis) |
| Logging        | `log/slog`, JSON to stdout               |
| Local infra    | Docker Compose                           |

## Architecture

```
            POST /webhooks, POST /events, GET /deliveries/{id}
                              │
                         ┌────▼────┐   1 txn: insert event +
 producers ─────────────►│   api   │── fan-out deliveries ──►┌────────────┐
                         └────┬────┘                         │ PostgreSQL │
                              │ ZADD delivery_id @ due_at    │  (truth)   │
                         ┌────▼────┐                         └─────▲──────┘
                         │  Redis  │  sorted set:                  │
                         │  ZSET   │  "what is due when"           │ lease, attempts,
                         └────┬────┘                               │ status updates
                              │ Lua pop-due (atomic)               │
                         ┌────▼─────────────────────────────────────┴─┐
                         │ worker  (N goroutines, bounded pool)       │
                         │  claim lease → sign → POST → record → retry│
                         └────────────────────┬───────────────────────┘
                                              │ HTTPS POST + Webhook-Signature
                                              ▼
                                     customer endpoints
```

**Flow**

1. `POST /events` validates the event. In a single transaction it inserts the event and one `deliveries` row for each enabled webhook subscribed to that type. After the commit it schedules each delivery ID in Redis and returns `202 Accepted` without waiting for any HTTP calls.
2. The worker pops due IDs from Redis with an atomic Lua script, then takes a **lease** on the row in Postgres (`locked_until`). If the lease isn't granted, another worker owns the delivery or it has already finished, so it is skipped.
3. The worker signs the body, POSTs it with a timeout, and writes a `delivery_attempts` row. A `2xx` response marks the delivery `succeeded`. Anything else schedules a retry with backoff, or marks the delivery `failed` once `max_attempts` is reached.
4. A **reconciler** (TODO) sweeps Postgres for due deliveries whose lease has expired and re-schedules them. This covers IDs lost from Redis, for example after a crash between pop and lease.

### Key design decisions

- **Postgres is the source of truth; Redis only holds hints.** If Redis lost all its data, the only effect would be delay: the reconciler rebuilds the schedule from `deliveries.next_attempt_at`. This avoids needing Redis to be durable or to share a transaction with Postgres.
- **At-least-once delivery.** A crash after the POST but before the result is recorded causes a redelivery. Each request carries a stable `Webhook-Id` (the delivery ID) so receivers can deduplicate.
- **Idempotent ingestion.** `event_id` is the primary key. Republishing the same event is a no-op (`200`), and the same ID with a different body is rejected with `409`. `UNIQUE(event_id, webhook_id)` stops fan-out from running twice.
- **Isolation between endpoints.** Each delivery takes one worker slot for at most `DELIVERY_TIMEOUT`, and retries are rescheduled rather than slept on, so a dead endpoint never blocks the others.
- **Exponential backoff with full jitter**, capped at `BACKOFF_MAX`, which avoids hitting an endpoint with a burst of retries as soon as it recovers.
- **Signed requests.** `Webhook-Signature: t=<unix>,v1=<hex HMAC-SHA256(secret, "<t>.<body>")>`. Including the timestamp lets receivers reject replays. Each webhook gets its own secret, which is returned only once, on creation.
- **Event payload stays opaque JSON** (`JSONB`). `design.md` mentions resolving runtime variables in payloads. That would be a rendering step in the worker before signing, and it isn't part of the core flow yet.

## Repository layout

```
cmd/
  api/         HTTP server entrypoint
  worker/      delivery worker entrypoint
  migrate/     applies SQL migrations and exits
internal/
  api/         router, middleware, error envelope, handlers (stubbed)
  config/      env-based configuration
  db/          pgx pool + embedded migration runner
    migrations/  versioned *.sql files (embedded into the binary)
  delivery/    worker loop, backoff (+ tests)
  domain/      shared types (Webhook, Event, Delivery, DeliveryAttempt)
  logging/     slog JSON logger
  queue/       Redis delay queue (ZSET + Lua pop)
  signing/     HMAC signing/verification (+ tests)
docs/
  failures.md  running log of failure modes / incidents found while building
```

## Prerequisites

- Go **1.26+**
- Docker Desktop with Compose v2, for Postgres and Redis. You can use your own Postgres 16+ and Redis 7+ instead.

## Setup and run

### Option A: run the Go binaries locally, with infra in Docker (recommended for development)

```bash
# 1. Start Postgres (host port 5433) and Redis (6379)
docker compose up -d postgres redis

# 2. Apply migrations
go run ./cmd/migrate

# 3. Start the API (terminal 1) and the worker (terminal 2)
go run ./cmd/api
go run ./cmd/worker

# 4. Check it is up
curl -s localhost:8080/healthz
# {"status":"ok"}
```

The defaults in [internal/config/config.go](internal/config/config.go) already match the compose services, so no environment variables are needed. To override a setting, export it in your shell. The binaries **do not** load `.env` automatically. [.env.example](.env.example) lists every variable.

PowerShell example:

```powershell
$env:LOG_LEVEL = "debug"; go run ./cmd/worker
```

### Option B: everything in Docker

```bash
docker compose --profile app up --build
```

This starts Postgres and Redis, runs `migrate` once, then starts `api` on `:8080` and `worker`.

### Tear down

```bash
docker compose --profile app down        # keep data
docker compose --profile app down -v     # also delete the Postgres volume
```

## Tests

```bash
go test ./...
```

Current coverage is a router test that checks 404 and 405 responses use the JSON envelope and the right `Allow` header, plus unit tests for signing (round trip, tampering, stale timestamp, malformed header) and backoff (growth, cap, jitter bounds). Integration tests against real Postgres and Redis (planned: testcontainers or the compose services) will cover:

- fan-out only to enabled, subscribed webhooks
- idempotent re-publish (200), and a conflicting re-publish (409)
- a success path against an `httptest.Server`
- timeouts, connection refused, and 5xx triggering retries, then `failed` after `max_attempts`
- two workers racing for the same delivery, where only one attempt is recorded
- a slow endpoint not delaying deliveries to a healthy one

## Configuration

| Variable             | Default                                                         | Meaning |
|----------------------|-----------------------------------------------------------------|---------|
| `HTTP_ADDR`          | `:8080`                                                         | API listen address |
| `DATABASE_URL`       | `postgres://webhooks:webhooks@localhost:5433/webhooks?sslmode=disable` | Postgres DSN |
| `REDIS_URL`          | `redis://localhost:6379/0`                                      | Redis URL |
| `LOG_LEVEL`          | `info`                                                          | `debug` / `info` / `warn` / `error` |
| `WORKER_CONCURRENCY` | `10`                                                            | Concurrent deliveries per worker process |
| `DELIVERY_TIMEOUT`   | `10s`                                                           | Total HTTP timeout per attempt |
| `MAX_ATTEMPTS`       | `8`                                                             | Attempts before a delivery is `failed` |
| `BACKOFF_BASE`       | `5s`                                                            | First retry ceiling |
| `BACKOFF_MAX`        | `1h`                                                            | Retry delay cap |
| `LEASE_DURATION`     | `60s`                                                           | Worker lease on a delivery row (must be > `DELIVERY_TIMEOUT`) |
| `POLL_INTERVAL`      | `500ms`                                                         | How often workers poll Redis for due work |

## API (planned contract)

Recorded requests and responses from the running Docker stack are in [docs/api-examples.md](docs/api-examples.md).

All errors share one envelope:

```json
{ "error": { "code": "validation_failed", "message": "...", "fields": { "url": "must be an absolute http(s) URL" } } }
```

### `POST /webhooks` → `201`

```bash
curl -s -X POST localhost:8080/webhooks -H 'Content-Type: application/json' -d '{
  "url": "https://acme.example.com/webhooks",
  "events": ["user.created", "payment.succeeded"]
}'
```

```json
{ "id": "6f1c…", "url": "https://acme.example.com/webhooks", "events": ["user.created","payment.succeeded"],
  "enabled": true, "secret": "whsec_…", "created_at": "…", "updated_at": "…" }
```

Errors: `400` for malformed JSON, `422` for a bad URL or an empty or invalid `events` list.

### `PATCH /webhooks/{id}` → `200`

```bash
curl -s -X PATCH localhost:8080/webhooks/<id> -H 'Content-Type: application/json' -d '{"enabled": false}'
```

`GET /webhooks` and `GET /webhooks/{id}` return the same shape, without `secret`.

### `POST /events` → `202`

```bash
curl -s -X POST localhost:8080/events -H 'Content-Type: application/json' -d '{
  "event_id": "evt_001",
  "type": "user.created",
  "timestamp": "2026-10-29T10:00:00Z",
  "data": { "user_id": "usr_456", "email": "test@example.com" }
}'
```

```json
{ "event_id": "evt_001", "deliveries_created": 2 }
```

`200` for an identical replay, `409` when the `event_id` already exists with a different body, `422` for validation errors.

### `GET /deliveries/{event_id}` → `200`

This departs from the brief's example on purpose. One event fans out to several webhooks, each with its own state, so the response gives an aggregate plus one entry per webhook:

```json
{
  "event_id": "evt_001",
  "status": "failed",
  "deliveries": [
    {
      "id": "d3a…", "webhook_id": "6f1c…", "status": "failed",
      "attempts": 3, "max_attempts": 3, "last_error": "HTTP 500", "last_status_code": 500,
      "next_retry_at": null, "created_at": "…", "updated_at": "…", "completed_at": "…"
    }
  ]
}
```

The aggregate `status` is the "worst" status across the event's deliveries: `failed` > `pending`/`in_progress` > `succeeded`.

### Outgoing webhook request

```
POST <webhook url>
Content-Type: application/json
Webhook-Id: <delivery id>          # stable across retries, use for dedup
Webhook-Event-Id: evt_001
Webhook-Signature: t=1792836000,v1=5f2c…

{"event_id":"evt_001","type":"user.created","timestamp":"2026-10-29T10:00:00Z","data":{…}}
```

Receivers verify the signature with the same algorithm as `signing.Verify` in [internal/signing/signing.go](internal/signing/signing.go).

## Database schema and migrations

The schema lives in [internal/db/migrations/0001_init.sql](internal/db/migrations/0001_init.sql):

| Table               | Purpose | Notable indexes / constraints |
|---------------------|---------|-------------------------------|
| `webhooks`          | Subscriptions: url, secret, `event_types TEXT[]`, enabled | partial GIN on `event_types WHERE enabled` for fan-out |
| `events`            | Ingested events; `id` is the client `event_id` | PK gives idempotency |
| `deliveries`        | One row per (event, webhook) with current state, lease, next retry | `UNIQUE(event_id, webhook_id)`; partial index on `next_attempt_at` for pending/in-progress |
| `delivery_attempts` | Append-only log of every HTTP attempt | `UNIQUE(delivery_id, attempt_number)` |

**How migrations work:** plain, forward-only SQL files are embedded into the binary. `cmd/migrate` applies any files not yet recorded in `schema_migrations`, in filename order, one transaction per file, under a Postgres advisory lock so that concurrent runs are safe. To add a migration, create `internal/db/migrations/000N_description.sql`. Don't edit a migration that has already been applied.

## Logging and monitoring

- Every log line is structured JSON with a `service` field (`api`, `worker` or `migrate`). HTTP request logs include method, path, status, duration and request ID.
- Planned metrics (Prometheus):
  - `deliveries_total{outcome}`
  - an attempt latency histogram
  - queue depth (`ZCARD`) and the age of the oldest due item
  - a reconciler re-enqueue count, which should stay near zero; a high count means Redis is losing work
  - `failed` deliveries per webhook
- Planned alerts:
  - oldest due item older than N minutes, meaning workers are behind
  - a spike in the failure rate
  - `/healthz` failing

## Implementation status

| Area | State |
|------|-------|
| Config, logging, graceful shutdown | done |
| Postgres pool, migration runner, schema | done |
| Redis delay queue (schedule / atomic pop) | done |
| Worker pool and poll loop | done |
| HMAC signing and verification, backoff | done, unit-tested |
| `GET /healthz` | done |
| Webhook CRUD handlers | TODO (`501`) |
| `POST /events` ingestion and fan-out | TODO (`501`) |
| `GET /deliveries/{event_id}` | TODO (`501`) |
| Worker `process` (lease → POST → record → retry) | TODO |
| Reconciler sweep | TODO |
| Integration tests | TODO |

## Assumptions and known trade-offs

- **At-least-once, not exactly-once.** Receivers must deduplicate on `Webhook-Id`.
- **No ordering guarantee** between events delivered to the same endpoint. Retries can reorder them.
- **Two stores instead of one.** A Postgres-only queue (`FOR UPDATE SKIP LOCKED`) would be simpler. Redis was chosen for cheap, precise delay scheduling. The cost is the reconciler and the fact that Redis and Postgres can briefly disagree.
- **Secrets are stored in plaintext** in the scaffold. In production they should be encrypted at rest (KMS), and rotation should support two active secrets.
- **No SSRF protection yet.** Before production, reject webhook URLs that resolve to private or link-local ranges, and re-check at connect time.
- No auth on the API itself. In production it would sit behind an API key or mTLS per tenant.

## Failure log

Failures, surprises and debugging notes found during development are recorded in [docs/failures.md](docs/failures.md).
