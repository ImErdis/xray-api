# xray-api

A control-plane REST API for running a commercial proxy/VPN service on top of
[Xray-core](https://github.com/XTLS/Xray-core). It is the application layer
Xray-core itself doesn't provide: **plans, subscription users, multi-node
provisioning, subscription links, traffic accounting, and quota enforcement** —
all driven over a clean HTTP API that your storefront, billing system, or
admin panel can call.

Built for ecommerce/SaaS: sell a plan, call `POST /users` (or let your billing
system hit the **signed webhook endpoint**), hand the customer a subscription
URL. The control plane pushes the account to every node, meters traffic,
auto-suspends on overage or expiry, and renews atomically on payment.

Highlights:

- **Billing integration** — provider-agnostic `POST /webhooks/billing`
  (HMAC-signed, replay-safe, idempotent provisioning) plus an atomic
  `POST /users/{id}/renew`; see [`docs/billing-webhooks.md`](docs/billing-webhooks.md)
- **Self-healing fleet** — per-node reconcilers converge nodes to the database
  state; Xray restarts are detected via uptime drop and users are re-pushed
  within one health interval
- **Accurate metering** — reset-on-read traffic deltas, quota auto-suspend,
  expiry sweeping
- **Operable** — Prometheus `/metrics`, per-IP rate limiting on public
  endpoints, structured logs, OpenAPI spec
- **Protocols** — VLESS (incl. `xtls-rprx-vision`), VMess, Trojan, Shadowsocks
  (AEAD), over tcp/ws/grpc/httpupgrade/**xhttp** with none/tls/reality; see
  [`docs/node-setup.md`](docs/node-setup.md)

## How it works

```
   storefront / admin ──HTTP──> xray-api ──gRPC──> Xray node (DE)
                                   │      ──gRPC──> Xray node (US)
                                   │      ──gRPC──> Xray node (SG)
                              PostgreSQL
                          (source of truth)
```

- Nodes are **stock Xray-core instances** with the gRPC API enabled — no agent
  is installed on them. The control plane uses `HandlerService` to add/remove
  users at runtime and `StatsService` to read per-user traffic.
- **PostgreSQL is the source of truth.** Because runtime-added Xray users do
  not survive an Xray restart, a per-node **reconciler** continuously converges
  each node to the desired state in the database (and re-pushes everything when
  a node comes back online).
- A **stats collector** polls each node every ~30s using reset-on-read counters
  (each read is an unambiguous delta), persists usage, and **auto-suspends**
  users who cross their data cap. An **expiry sweeper** does the same for
  time-limited plans.

See [`docs/node-setup.md`](docs/node-setup.md) and
[`docs/security.md`](docs/security.md) for the node side.

## Quick start (local end-to-end)

Requires Docker. This brings up the API, PostgreSQL, **and a real Xray node**:

```bash
docker compose -f deploy/docker-compose.dev.yml up --build
```

Then provision a user (the dev stack ships a bootstrap key `dev-bootstrap-key`):

```bash
KEY=dev-bootstrap-key
api() { curl -s -H "Authorization: Bearer $KEY" -H 'Content-Type: application/json' "$@"; }
B=localhost:8080/api/v1

# A sellable plan: 100 GB for 30 days.
PLAN=$(api -X POST $B/plans -d '{"name":"pro","data_limit_bytes":107374182400,"duration_days":30}')
PLAN_ID=$(echo "$PLAN" | jq -r .id)

# Register the dockerized node (reachable as xray:10085 on the compose network).
NODE=$(api -X POST $B/nodes -d '{"name":"local","api_address":"xray","api_port":10085}')
NODE_ID=$(echo "$NODE" | jq -r .id)

# Register the managed inbound (tag must match the node's Xray config).
IB=$(api -X POST $B/nodes/$NODE_ID/inbounds -d '{
  "tag":"vless-ws","protocol":"vless","listen_port":443,
  "public_host":"localhost","public_port":443,
  "network":"ws","security":"tls","ws_path":"/ws","sni":"localhost"}')
IB_ID=$(echo "$IB" | jq -r .id)

# Sell it: create the user, get the subscription URL.
api -X POST $B/users -d "{\"email\":\"alice@example.com\",\"plan_id\":\"$PLAN_ID\",\"inbound_ids\":[\"$IB_ID\"]}" | jq .
```

Import the returned `sub_url` into v2rayN / Clash Verge. Watch usage with
`GET /api/v1/users/{id}/usage`; set a tiny `data_limit_bytes` to see
auto-suspend.

## Running in production

1. Build the image: `make docker` (or use `deploy/docker-compose.yml` which runs
   the API + PostgreSQL).
2. Configure via `examples/config.example.yaml` or `XRAY_API_*` env vars.
3. Create an admin key: `xray-api gen-key -name admin` (or set
   `auth.bootstrap_api_key` for first run).
4. Put the API behind TLS (reverse proxy). Keep node gRPC ports off the public
   internet — see [`docs/security.md`](docs/security.md).

## API surface

`Authorization: Bearer <key>` (or `X-API-Key`) on all `/api/v1/*` routes. Full
spec: [`api/openapi.yaml`](api/openapi.yaml), also served at
`GET /api/v1/openapi.yaml`.

| Area | Endpoints |
|---|---|
| Nodes | `GET/POST /nodes`, `GET/PATCH/DELETE /nodes/{id}`, `POST /nodes/{id}/reconcile` |
| Inbounds | `GET/POST /nodes/{id}/inbounds`, `GET/PATCH/DELETE /inbounds/{id}` |
| Plans | `GET/POST /plans`, `GET/PATCH/DELETE /plans/{id}` |
| Users | `GET/POST /users`, `GET/PATCH/DELETE /users/{id}`, `PUT /users/{id}/inbounds`, `POST /users/{id}/{renew,suspend,resume,reset-traffic,rotate-sub-token}`, `GET /users/{id}/usage` |
| API keys | `GET/POST /api-keys`, `DELETE /api-keys/{id}` |
| Subscription (public) | `GET /sub/{token}` (`?format=v2ray\|clash`), `GET /sub/{token}/info` |
| Billing (public, signed) | `POST /webhooks/billing` — see [`docs/billing-webhooks.md`](docs/billing-webhooks.md) |
| Ops | `GET /metrics` (Prometheus), `GET /healthz` |

## Development

```bash
make build           # static binary -> bin/xray-api
make test            # unit tests (store tests skip without TEST_PG_DSN)
make vet
make integration     # live tests against a real Xray node (see test/integration)
```

CI (GitHub Actions) runs gofmt/vet/build/test with a PostgreSQL service so the
store layer is exercised against real Postgres on every push.

### Layout

```
cmd/xray-api      entrypoint (serve, gen-key, version)
internal/
  config          YAML + env config
  domain          core types (node, inbound, plan, user, apikey)
  store           PostgreSQL repository + embedded goose migrations
  xray            the ONLY boundary to xtls/xray-core (gRPC client, accounts)
  service         business logic (provisioning, lifecycle, sub rendering)
  worker          per-node actors: health, reconcile, stats; expiry sweeper
  sublink         vless/vmess/trojan links, v2ray + Clash output, userinfo header
  httpapi         chi router, handlers, auth
api               OpenAPI spec (embedded)
docs              node setup, security, example node config
deploy            Dockerfile, compose (prod + dev-with-xray)
```

## License

MIT — see [LICENSE](LICENSE).
