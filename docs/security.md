# Security notes

## The Xray gRPC API is unauthenticated — never expose it publicly

Xray-core's command API (`HandlerService`/`StatsService`) has **no built-in
authentication**. Anyone who can open a TCP connection to that port can add or
remove users and read traffic stats. Treat the gRPC port exactly like a
database port.

Acceptable ways to let `xray-api` reach a node's gRPC port:

1. **Private network** — control plane and nodes share a VPC/LAN; the port
   binds to a private interface only.
2. **WireGuard / VPN mesh** — nodes join an overlay network; register the node
   with its overlay IP.
3. **SSH tunnel** — forward the remote 10085 to a local port.
4. **TLS** — terminate TLS on the `api-in` inbound and register the node with
   `api_tls: true` (and `api_tls_insecure: true` for self-signed certs). This
   protects confidentiality/integrity in transit but still does not
   authenticate the *client*; combine with network ACLs. Mutual TLS is planned.

The bundled `docs/example-node-config.json` binds the API inbound to
`127.0.0.1` so that, by default, it is unreachable from outside the host until
you deliberately choose one of the above.

## Admin API authentication

All `/api/v1/*` routes require an API key via `Authorization: Bearer <key>` or
`X-API-Key`. Keys are stored only as SHA-256 hashes; the plaintext is shown
once at creation (`xray-api gen-key` or `POST /api/v1/api-keys`). Revoke
compromised keys with `DELETE /api/v1/api-keys/{id}`.

The `auth.bootstrap_api_key` config value is a convenience for first-run setup.
Prefer issuing real keys and leaving it empty in production.

## Subscription tokens

`GET /sub/{token}` is public by design (clients fetch it unauthenticated). The
token is 32 bytes of CSPRNG entropy. If a token leaks, rotate it with
`POST /api/v1/users/{id}/rotate-sub-token`, which invalidates old links.

## Transport for the admin API

Terminate TLS in front of `xray-api` (reverse proxy) for any non-loopback
deployment, so API keys and subscription tokens are not sent in clear text.
