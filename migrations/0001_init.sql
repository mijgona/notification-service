-- +goose Up
CREATE TABLE notifications (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    idempotency_key TEXT NOT NULL UNIQUE,
    channel         TEXT NOT NULL CHECK (channel IN ('telegram', 'email')),
    recipient       TEXT NOT NULL,
    subject         TEXT NOT NULL DEFAULT '',
    body            TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending', 'processing', 'delivered', 'failed')),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE outbox (
    id              BIGSERIAL PRIMARY KEY,
    notification_id UUID NOT NULL REFERENCES notifications (id),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    processed_at    TIMESTAMPTZ
);

CREATE INDEX idx_outbox_unprocessed ON outbox (id) WHERE processed_at IS NULL;

CREATE TABLE delivery_attempts (
    id              BIGSERIAL PRIMARY KEY,
    notification_id UUID NOT NULL REFERENCES notifications (id),
    attempt_no      INT NOT NULL,
    status          TEXT NOT NULL CHECK (status IN ('success', 'error')),
    error           TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_attempts_notification ON delivery_attempts (notification_id);

-- +goose Down
DROP TABLE delivery_attempts;
DROP TABLE outbox;
DROP TABLE notifications;
