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
4. **TLS with CA pinning** — terminate TLS on the `api-in` inbound and register
   the node with `api_tls: true` plus `api_ca_cert` (the PEM that signed the
   node's server cert). The control plane then verifies the node's identity
   against your CA rather than the system roots, and against
   `api_tls_server_name`. Use `api_tls_insecure: true` only to skip
   verification in throwaway setups.
5. **Mutual TLS (mTLS)** — additionally register `api_client_cert` and
   `api_client_key` (PEM). The control plane presents this client certificate
   so the node side can authenticate *it*, closing the "anyone who reaches the
   port can manage users" gap. The node enforces client-cert verification
   either via a TLS-terminating reverse proxy in front of the API inbound
   (e.g. nginx `ssl_verify_client on` / `ssl_client_certificate ca.pem`) or any
   gateway that does mTLS; the control-plane side is identical regardless.

### mTLS fields on a node

| Field | Meaning |
|---|---|
| `api_tls` | enable TLS for the gRPC connection |
| `api_tls_server_name` | expected server certificate name (SNI + verification) |
| `api_ca_cert` | PEM CA that signed the node's server cert (pinning) |
| `api_client_cert` | PEM client certificate the control plane presents (mTLS) |
| `api_client_key` | PEM private key for the client cert — **write-only**, never returned by the API; `api_has_client_key` reports whether one is stored |
| `api_tls_insecure` | skip server verification (testing only) |

Cert/CA material is validated when the node is created or updated (malformed
PEM or a cert/key mismatch is rejected with `400`), and the private key is
stored but never serialized back. Omit the cert fields on a `PATCH` to leave
them unchanged; send `""` to clear one.

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
