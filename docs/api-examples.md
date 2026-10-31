# API run examples

These are real requests made against the Docker stack on 2026-10-29, with the responses that came back. `Date` and `Content-Length` headers are left out. For the intended request and response shapes once the handlers are implemented, see [API (planned contract)](../README.md#api-planned-contract) in the README.

> The scaffold's handlers are still stubs, so the business endpoints currently return `501`. Re-run these examples and update this file as each handler is implemented.

## 1. Build the image and start the stack

```bash
# Build the single image that contains all three binaries (api, worker, migrate)
docker build -t rival-webhooks:dev .
# => rival-webhooks:dev   78.6MB on disk, 25.6MB content

# Start Postgres and Redis, run migrations once, then start the api and the worker
docker compose --profile app up -d --build
docker compose --profile app ps -a
```

```
NAME               SERVICE    STATUS
rival-api-1        api        Up            0.0.0.0:8080->8080/tcp
rival-migrate-1    migrate    Exited (0)
rival-postgres-1   postgres   Up (healthy)  0.0.0.0:5433->5432/tcp
rival-redis-1      redis      Up (healthy)  0.0.0.0:6379->6379/tcp
rival-worker-1     worker     Up
```

Startup logs (JSON, one line per event):

```json
{"level":"INFO","msg":"applied migration","service":"migrate","version":"migrations/0001_init.sql"}
{"level":"INFO","msg":"migrations up to date","service":"migrate"}
{"level":"INFO","msg":"worker started","service":"worker","concurrency":10}
{"level":"INFO","msg":"api listening","service":"api","addr":":8080"}
```

To check the schema:

```bash
docker compose exec postgres psql -U webhooks -c '\dt'
# deliveries, delivery_attempts, events, schema_migrations, webhooks
```

## 2. Health check

```bash
curl -i localhost:8080/healthz
```

```
HTTP/1.1 200 OK
Content-Type: application/json

{"status":"ok"}
```

With a dependency down (`docker compose stop redis`):

```
HTTP/1.1 503 Service Unavailable
Content-Type: application/json

{"error":{"code":"unavailable","message":"redis: context deadline exceeded"}}
```

Each dependency ping is limited to 2 s, so this returns in about 2 s. Postgres being down gives the same response with a `postgres:` message. Before the fix it took about 15 s; see [failures.md](failures.md).

## 3. Register a webhook: `POST /webhooks`

```bash
curl -i -X POST localhost:8080/webhooks -H 'Content-Type: application/json' -d '{
  "url": "https://acme.example.com/webhooks",
  "events": ["user.created", "payment.succeeded"]
}'
```

```
HTTP/1.1 501 Not Implemented
Content-Type: application/json

{"error":{"code":"not_implemented","message":"endpoint not implemented yet"}}
```

`GET /webhooks`, `GET /webhooks/{id}` and `PATCH /webhooks/{id}` (for example `-d '{"enabled": false}'`) currently return the same `501` body.

## 4. Publish an event: `POST /events`

```bash
curl -i -X POST localhost:8080/events -H 'Content-Type: application/json' -d '{
  "event_id": "evt_001",
  "type": "user.created",
  "timestamp": "2026-10-29T10:00:00Z",
  "data": { "user_id": "usr_456", "email": "test@example.com" }
}'
```

```
HTTP/1.1 501 Not Implemented
Content-Type: application/json

{"error":{"code":"not_implemented","message":"endpoint not implemented yet"}}
```

## 5. Inspect deliveries: `GET /deliveries/{event_id}`

```bash
curl -i localhost:8080/deliveries/evt_001
```

```
HTTP/1.1 501 Not Implemented
Content-Type: application/json

{"error":{"code":"not_implemented","message":"endpoint not implemented yet"}}
```

## 6. Router behaviour

Unknown paths and wrong methods use the same JSON error envelope as every other error:

```
$ curl -i localhost:8080/nope
HTTP/1.1 404 Not Found
Content-Type: application/json

{"error":{"code":"not_found","message":"no route for /nope"}}
```

A 405 also lists the methods that are allowed:

```
$ curl -i -X DELETE localhost:8080/events
HTTP/1.1 405 Method Not Allowed
Allow: POST
Content-Type: application/json

{"error":{"code":"method_not_allowed","message":"DELETE not allowed on /events"}}
```

Every request is logged with its status, duration and request ID:

```json
{"level":"INFO","msg":"http request","service":"api","method":"GET","path":"/deliveries/evt_001","status":501,"duration_ms":0,"request_id":"9f38822b9d75/65PeqBZyY7-000006"}
```

## Tear down

```bash
docker compose --profile app down      # keep the data
docker compose --profile app down -v   # also delete the Postgres volume
```
