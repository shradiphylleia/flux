# Failure log

A running record of failures hit while building or running the service: what broke, why, and what changed. Add new entries at the top.

## Template

```
### YYYY-MM-DD — <short title>
- **Symptom:** what was observed (error message, log line, failing test)
- **Cause:** root cause once understood
- **Fix / decision:** what changed (link commit/file)
- **Follow-up:** anything left to do
```

## Known failure modes to design for

| Failure | Expected behaviour |
|---------|--------------------|
| Endpoint times out | Attempt recorded with `error="timeout"`; retried with backoff |
| Connection refused / DNS failure | Same as timeout, with the specific error text stored |
| Non-2xx response (incl. 3xx) | Attempt recorded with `status_code`; retried |
| Max attempts reached | Delivery marked `failed`, `next_attempt_at = NULL` |
| Worker crashes mid-delivery | Lease expires; reconciler re-schedules; endpoint may receive a duplicate |
| Redis loses data / restarts | Reconciler rebuilds schedule from Postgres; only delay, no loss |
| Webhook disabled while delivery pending | Worker checks `enabled` before sending; marks delivery `failed` ("webhook disabled") |
| Same `event_id` published twice | `200` no-op if identical, `409` if body differs; no duplicate deliveries |

---

## Entries

### 2026-10-29 — `/healthz` hangs 15 s when Redis is down
- **Symptom:** With `docker compose stop redis`, `GET /healthz` took about 15 s to return `503 {"error":{"code":"unavailable","message":"redis: context deadline exceeded"}}`.
- **Cause:** The ping uses the request context, so only the router's `middleware.Timeout(15s)` ends it. Health probes usually time out well before 15 s, so the orchestrator would record "no response" instead of the 503.
- **Fix / decision:** `healthz` now wraps the pings in `context.WithTimeout(r.Context(), healthCheckTimeout)` (2 s) ([internal/api/server.go](../internal/api/server.go)). Checked against the live stack: Redis down or Postgres down now returns `503` in about 2.0 s.
- **Follow-up:** None.

### 2026-10-29 — 404/405 responses bypassed the JSON error envelope
- **Symptom:** `GET /nope` returned chi's plain-text `404 page not found`, and the 405 had an empty body. Every other error uses `{"error":{...}}`.
- **Cause:** chi's default `NotFound` and `MethodNotAllowed` handlers were in use.
- **Fix / decision:** Custom handlers now write the JSON envelope (`not_found`, `method_not_allowed`). A custom 405 handler doesn't get chi's allowed-method list, which is unexported, so the handler rebuilds the `Allow` header by probing the router with `Mux.Match`. Covered by `TestRouterErrorsUseJSONEnvelope` in [internal/api/server_test.go](../internal/api/server_test.go).

### 2026-10-29 — Local Postgres already on 5432
- **Symptom:** A Postgres 17 install on the dev machine already binds `5432`.
- **Fix / decision:** Compose maps the container to host port `5433`, and the default `DATABASE_URL` points at `5433`.
