-- 0001_init: core schema for webhook registration, events, and deliveries.
-- Postgres is the source of truth; Redis only holds scheduling hints.

CREATE TABLE webhooks (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    url          TEXT        NOT NULL,
    secret       TEXT        NOT NULL,           -- HMAC signing secret, returned once on create
    event_types  TEXT[]      NOT NULL CHECK (cardinality(event_types) > 0),
    enabled      BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Fan-out lookup: "active webhooks subscribed to type X" => event_types @> ARRAY[X] AND enabled.
CREATE INDEX webhooks_event_types_active_idx ON webhooks USING GIN (event_types) WHERE enabled;

CREATE TABLE events (
    id           TEXT        PRIMARY KEY,         -- client-supplied event_id; doubles as idempotency key
    type         TEXT        NOT NULL,
    occurred_at  TIMESTAMPTZ NOT NULL,            -- client "timestamp"
    payload      JSONB       NOT NULL,
    received_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX events_type_received_idx ON events (type, received_at DESC);

CREATE TYPE delivery_status AS ENUM ('pending', 'in_progress', 'succeeded', 'failed');

-- One row per (event, webhook) pair. Tracks the current state of that delivery.
CREATE TABLE deliveries (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id         TEXT            NOT NULL REFERENCES events (id) ON DELETE CASCADE,
    webhook_id       UUID            NOT NULL REFERENCES webhooks (id) ON DELETE CASCADE,
    status           delivery_status NOT NULL DEFAULT 'pending',
    attempt_count    INT             NOT NULL DEFAULT 0,
    max_attempts     INT             NOT NULL,
    next_attempt_at  TIMESTAMPTZ,                 -- NULL once terminal
    locked_until     TIMESTAMPTZ,                 -- worker lease; guards against duplicate processing
    last_error       TEXT,
    last_status_code INT,
    created_at       TIMESTAMPTZ     NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ     NOT NULL DEFAULT now(),
    completed_at     TIMESTAMPTZ,
    UNIQUE (event_id, webhook_id)                 -- re-publishing an event never duplicates fan-out
);

-- Reconciler sweep: find due work that Redis may have lost.
CREATE INDEX deliveries_due_idx ON deliveries (next_attempt_at)
    WHERE status IN ('pending', 'in_progress');
CREATE INDEX deliveries_webhook_idx ON deliveries (webhook_id, created_at DESC);

-- Append-only log of every HTTP attempt.
CREATE TABLE delivery_attempts (
    id               BIGSERIAL PRIMARY KEY,
    delivery_id      UUID        NOT NULL REFERENCES deliveries (id) ON DELETE CASCADE,
    attempt_number   INT         NOT NULL,
    started_at       TIMESTAMPTZ NOT NULL,
    finished_at      TIMESTAMPTZ NOT NULL,
    status_code      INT,                         -- NULL on timeout / connection error
    error            TEXT,
    response_snippet TEXT,                        -- first N bytes of the body, for debugging
    UNIQUE (delivery_id, attempt_number)
);
