-- +goose Up
-- Shadowsocks cipher (inbound-level; shared by all users on the inbound) and
-- XHTTP transport mode. Both nullable/defaulted so existing inbounds are
-- unaffected.
ALTER TABLE inbounds ADD COLUMN method TEXT NOT NULL DEFAULT '';
ALTER TABLE inbounds ADD COLUMN xhttp_mode TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE inbounds DROP COLUMN xhttp_mode;
ALTER TABLE inbounds DROP COLUMN method;
