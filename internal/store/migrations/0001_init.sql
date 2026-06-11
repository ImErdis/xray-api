-- +goose Up
CREATE TABLE nodes (
    id                  UUID PRIMARY KEY,
    name                TEXT NOT NULL UNIQUE,
    api_address         TEXT NOT NULL,
    api_port            INTEGER NOT NULL,
    api_tls             BOOLEAN NOT NULL DEFAULT FALSE,
    api_tls_server_name TEXT NOT NULL DEFAULT '',
    api_tls_insecure    BOOLEAN NOT NULL DEFAULT FALSE,
    status              TEXT NOT NULL DEFAULT 'unknown',
    last_seen_at        TIMESTAMPTZ,
    last_error          TEXT NOT NULL DEFAULT '',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE inbounds (
    id                 UUID PRIMARY KEY,
    node_id            UUID NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    tag                TEXT NOT NULL,
    protocol           TEXT NOT NULL,
    listen_port        INTEGER NOT NULL,
    public_host        TEXT NOT NULL,
    public_port        INTEGER NOT NULL,
    network            TEXT NOT NULL DEFAULT 'tcp',
    security           TEXT NOT NULL DEFAULT 'none',
    ws_path            TEXT NOT NULL DEFAULT '',
    host_header        TEXT NOT NULL DEFAULT '',
    grpc_service_name  TEXT NOT NULL DEFAULT '',
    sni                TEXT NOT NULL DEFAULT '',
    fingerprint        TEXT NOT NULL DEFAULT '',
    reality_public_key TEXT NOT NULL DEFAULT '',
    reality_short_id   TEXT NOT NULL DEFAULT '',
    flow               TEXT NOT NULL DEFAULT '',
    remark             TEXT NOT NULL DEFAULT '',
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (node_id, tag)
);

CREATE TABLE plans (
    id               UUID PRIMARY KEY,
    name             TEXT NOT NULL UNIQUE,
    data_limit_bytes BIGINT,
    duration_days    INTEGER,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE users (
    id                  UUID PRIMARY KEY,
    email               TEXT NOT NULL UNIQUE,
    uuid                UUID NOT NULL UNIQUE,
    trojan_password     TEXT NOT NULL UNIQUE,
    plan_id             UUID REFERENCES plans(id) ON DELETE SET NULL,
    status              TEXT NOT NULL DEFAULT 'active',
    data_limit_bytes    BIGINT,
    used_upload_bytes   BIGINT NOT NULL DEFAULT 0,
    used_download_bytes BIGINT NOT NULL DEFAULT 0,
    expires_at          TIMESTAMPTZ,
    sub_token           TEXT NOT NULL UNIQUE,
    note                TEXT NOT NULL DEFAULT '',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_users_status ON users(status);
CREATE INDEX idx_users_expires_at ON users(expires_at) WHERE expires_at IS NOT NULL;

CREATE TABLE user_inbounds (
    user_id        UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    inbound_id     UUID NOT NULL REFERENCES inbounds(id) ON DELETE CASCADE,
    sync_status    TEXT NOT NULL DEFAULT 'pending',
    last_synced_at TIMESTAMPTZ,
    last_error     TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (user_id, inbound_id)
);
CREATE INDEX idx_user_inbounds_inbound ON user_inbounds(inbound_id);
CREATE INDEX idx_user_inbounds_sync_status ON user_inbounds(sync_status);

CREATE TABLE traffic_snapshots (
    id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id        UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    node_id        UUID NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    uplink_bytes   BIGINT NOT NULL,
    downlink_bytes BIGINT NOT NULL,
    collected_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_traffic_user_time ON traffic_snapshots(user_id, collected_at);
CREATE INDEX idx_traffic_collected_at ON traffic_snapshots(collected_at);

CREATE TABLE api_keys (
    id           UUID PRIMARY KEY,
    name         TEXT NOT NULL,
    key_hash     TEXT NOT NULL UNIQUE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_used_at TIMESTAMPTZ,
    revoked_at   TIMESTAMPTZ
);

-- +goose Down
DROP TABLE api_keys;
DROP TABLE traffic_snapshots;
DROP TABLE user_inbounds;
DROP TABLE users;
DROP TABLE plans;
DROP TABLE inbounds;
DROP TABLE nodes;
