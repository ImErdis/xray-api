-- +goose Up
-- Link users to external billing systems (Stripe customer/subscription id,
-- WooCommerce order id, ...). Unique so webhook provisioning is idempotent.
ALTER TABLE users ADD COLUMN external_id TEXT UNIQUE;

-- Processed webhook events, for replay-safe idempotency. id is the provider's
-- event id (e.g. Stripe evt_...).
CREATE TABLE webhook_events (
    id          TEXT PRIMARY KEY,
    action      TEXT NOT NULL,
    received_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE webhook_events;
ALTER TABLE users DROP COLUMN external_id;
