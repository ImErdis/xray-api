-- +goose Up
-- mTLS material for connecting to a node's gRPC API: a pinned CA to verify the
-- node's server certificate, and a client certificate/key the control plane
-- presents so the node can authenticate it. All PEM-encoded; empty by default.
ALTER TABLE nodes ADD COLUMN api_ca_cert TEXT NOT NULL DEFAULT '';
ALTER TABLE nodes ADD COLUMN api_client_cert TEXT NOT NULL DEFAULT '';
ALTER TABLE nodes ADD COLUMN api_client_key TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE nodes DROP COLUMN api_client_key;
ALTER TABLE nodes DROP COLUMN api_client_cert;
ALTER TABLE nodes DROP COLUMN api_ca_cert;
