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

## Supported protocols and transports

What the control plane can **provision and render into subscription links**:

| Protocol | Notes |
|---|---|
| VLESS | incl. `xtls-rprx-vision` flow (tcp+tls/reality) |
| VMess | AEAD (alterId 0) |
| Trojan | password auth |
| Shadowsocks | AEAD ciphers: `aes-128-gcm`, `aes-256-gcm`, `chacha20-ietf-poly1305`, `xchacha20-ietf-poly1305`, `none`. The cipher is an inbound-level `method` shared by all users; register it on the inbound. |

| Transport (`network`) | Security (`security`) |
|---|---|
| `tcp`, `ws`, `grpc`, `httpupgrade`, `xhttp` | `none`, `tls`, `reality` |

For **XHTTP**, set `network: xhttp`, the `ws_path` field as the XHTTP path, and
optionally `xhttp_mode` (`auto`/`packet-up`/`stream-up`/`stream-one`). XHTTP is
the current recommended CDN-friendly transport (WebSocket is deprecated
upstream).

Anything else Xray supports (WireGuard, mKCP, SOCKS/HTTP, ECH) still runs fine
on the node — the control plane just doesn't provision users into it. Routing,
outbounds, DNS, fallbacks, and balancers are owned entirely by the node's own
config file and are not touched by the API.

### Shadowsocks inbound example

```json
{ "tag": "ss-multi", "port": 8388, "protocol": "shadowsocks",
  "settings": { "clients": [], "network": "tcp,udp" } }
```

Register it with `"protocol":"shadowsocks","method":"aes-256-gcm"`. Each user's
generated password is pushed at runtime.

### VLESS over XHTTP inbound example

```json
{ "tag": "vless-xhttp", "port": 2096, "protocol": "vless",
  "settings": { "clients": [], "decryption": "none" },
  "streamSettings": { "network": "xhttp", "security": "tls",
    "xhttpSettings": { "path": "/xh", "mode": "auto" },
    "tlsSettings": { "...": "..." } } }
```

## Connecting the gRPC API over TLS (optional)

To avoid a private network, add a TLS `streamSettings` block to the `api-in`
inbound and register the node with `"api_tls": true`. For self-signed certs set
`"api_tls_insecure": true`. Mutual TLS is not yet supported.
