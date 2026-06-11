# Preparing an Xray node

`xray-api` manages **stock Xray-core instances** — no agent is installed on the
node. The control plane drives each node through Xray's gRPC API
(`HandlerService` to add/remove users, `StatsService` to read traffic). A node
is "manageable" once its config enables that API and defines the inbounds whose
client lists the control plane should own.

See [`example-node-config.json`](./example-node-config.json) for a complete,
working config. The essential pieces:

## 1. Enable the gRPC API

```json
"api": { "tag": "api", "services": ["HandlerService", "StatsService"] },
"stats": {}
```

## 2. Enable per-user traffic counters

Per-user stats are keyed by each client's `email`, and only recorded when the
policy enables them:

```json
"policy": {
  "levels": { "0": { "statsUserUplink": true, "statsUserDownlink": true } },
  "system": { "statsInboundUplink": true, "statsInboundDownlink": true }
}
```

`xray-api` sets each provisioned user's `email` to the user's unique email in
the control plane, so counters appear as
`user>>>{email}>>>traffic>>>uplink|downlink`.

## 3. Expose the API on a dokodemo-door inbound, routed to the api tag

```json
"inbounds": [
  { "tag": "api-in", "listen": "127.0.0.1", "port": 10085,
    "protocol": "dokodemo-door", "settings": { "address": "127.0.0.1" } }
],
"routing": { "rules": [
  { "type": "field", "inboundTag": ["api-in"], "outboundTag": "api" }
] }
```

**Security:** `listen` is `127.0.0.1` here on purpose. The gRPC API has no
authentication of its own — anyone who can reach port 10085 can add/remove
users. Never expose it on a public interface. Reach it over a private network,
WireGuard, or an SSH tunnel, or wrap it in TLS (see below). See
[`security.md`](./security.md).

## 4. Define the proxy inbounds with empty client lists

```json
{
  "tag": "vless-ws", "port": 443, "protocol": "vless",
  "settings": { "clients": [], "decryption": "none" },
  "streamSettings": { "network": "ws", "security": "tls", "...": "..." }
}
```

Leave `clients` empty. The control plane is the single source of truth for
membership and adds/removes clients at runtime. **The inbound `tag`
(`vless-ws`) is the link between node and control plane** — when you register
the inbound via the API you must use the same tag.

## 5. Register the node and its inbounds

```bash
# Register the node (point at the gRPC API endpoint).
curl -sX POST localhost:8080/api/v1/nodes \
  -H "Authorization: Bearer $KEY" -H 'Content-Type: application/json' \
  -d '{"name":"de-1","api_address":"10.0.0.2","api_port":10085}'

# Register each managed inbound. `tag` must match the node config; the
# public_host/public_port are what clients connect to (the CDN/edge address).
curl -sX POST localhost:8080/api/v1/nodes/$NODE_ID/inbounds \
  -H "Authorization: Bearer $KEY" -H 'Content-Type: application/json' \
  -d '{"tag":"vless-ws","protocol":"vless","listen_port":443,
       "public_host":"de1.example.com","public_port":443,
       "network":"ws","security":"tls","ws_path":"/ws","sni":"de1.example.com"}'
```

## Connecting the gRPC API over TLS (optional)

To avoid a private network, add a TLS `streamSettings` block to the `api-in`
inbound and register the node with `"api_tls": true`. For self-signed certs set
`"api_tls_insecure": true`. Mutual TLS is not yet supported.
