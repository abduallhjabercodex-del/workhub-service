-- 0003_outbox.up.sql
BEGIN;

CREATE TABLE outbox_events (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    event_id        TEXT NOT NULL UNIQUE,
    event_type      TEXT NOT NULL,
    aggregate_type  TEXT NOT NULL,
    aggregate_id    BIGINT NOT NULL,
    payload         JSONB NOT NULL,
    attempts        INTEGER NOT NULL DEFAULT 0,
    available_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at    TIMESTAMPTZ,
    last_error      TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT outbox_event_type_not_blank     CHECK (btrim(event_type) <> ''),
    CONSTRAINT outbox_aggregate_type_not_blank CHECK (btrim(aggregate_type) <> ''),
    CONSTRAINT outbox_attempts_non_negative    CHECK (attempts >= 0)
);

-- Partial index: يجلب الأحداث غير المنشورة فقط
CREATE INDEX outbox_unpublished_idx
    ON outbox_events (available_at, id)
    WHERE published_at IS NULL;

-- ============================================================
-- notification_deliveries
-- يستخدمه Notification Worker لتجنب التسليم المزدوج (Idempotency).
-- ============================================================
CREATE TABLE notification_deliveries (
    event_id      TEXT PRIMARY KEY,
    delivered_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMIT;
