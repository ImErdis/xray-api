# Billing webhooks

`POST /webhooks/billing` lets any billing system drive the full subscription
lifecycle — provision on purchase, renew on payment, suspend on failed payment,
cancel on subscription end — through one signed endpoint. It is
provider-agnostic: anything that can POST JSON with an HMAC header works
(Stripe via a thin relay, WooCommerce, crypto gateways, your own store backend).

## Setup

Set a shared secret (the endpoint returns 404 until one is configured):

```yaml
webhooks:
  billing_secret: "whsec_change_me"   # or XRAY_API_WEBHOOKS_BILLING_SECRET
```

## Request format

- `Content-Type: application/json`
- `X-Webhook-Signature: <hex HMAC-SHA256 of the raw request body, keyed with the secret>`

```bash
BODY='{"event_id":"evt_123", ...}'
SIG=$(printf '%s' "$BODY" | openssl dgst -sha256 -hmac "$SECRET" -hex | sed 's/^.*= //')
curl -X POST https://panel.example.com/webhooks/billing \
  -H "X-Webhook-Signature: $SIG" -d "$BODY"
```

## Event payload

```jsonc
{
  "event_id": "evt_123",        // REQUIRED, provider-unique. Replays of the
                                // same id are acknowledged but not re-executed.
  "action": "provision",        // provision|renew|suspend|resume|cancel

  // User reference (renew/suspend/resume/cancel): one of
  "external_id": "sub_stripe_abc",  // preferred — set at provision time
  "email": "alice@example.com",

  // action=provision
  "provision": {
    "email": "alice@example.com",
    "plan_id": "…",                 // optional; snapshots the plan's limits
    "inbound_ids": ["…"],           // required
    "data_limit_bytes": 107374182400,  // optional override
    "expires_at": "2026-07-01T00:00:00Z",  // optional override
    "note": "order #1001"
  },

  // action=renew
  "renew": {
    "days": 30,             // extends from current expiry if in the future,
                            // else from now (early renewals stack)
    "reset_traffic": true,  // typical for monthly data quotas
    "plan_id": "…"          // optional plan switch (re-snapshots data limit)
  }
}
```

### Semantics

| Action | Effect |
|---|---|
| `provision` | Creates the user with `external_id` and pushes to nodes. Idempotent: an existing user with the same `external_id` is returned unchanged (safe across provider retries with fresh event ids). |
| `renew` | Extends expiry / resets traffic / switches plan, reactivates, re-pushes. |
| `suspend` | Disables the user and removes them from all nodes. Record, stats, and subscription token are kept. |
| `resume` | Reactivates (refused while over quota or expired — renew instead). |
| `cancel` | Same as suspend; a later `renew` or `resume` restores service without re-provisioning. |

Responses: `200` with `{"action", "duplicate", "user"?}`; `401` on bad
signature; `400` on malformed events; `404` for unknown users.

## Idempotency — two layers

1. **`event_id`** — stored on first sight; replays return `{"duplicate": true}`
   without executing. Use the provider's event id (Stripe `evt_…`).
2. **`external_id`** — provisioning is keyed on it, so even a *new* event id
   for the same purchase cannot create a second account.

## Wiring up Stripe

Stripe signs its own webhooks differently, so don't point Stripe at this
endpoint directly — use a ~20-line relay (serverless function or a route in
your store backend) that verifies Stripe's signature, then forwards:

| Stripe event | Forward as |
|---|---|
| `checkout.session.completed` | `provision` with `external_id` = subscription id, plan/inbounds from your price-id mapping |
| `invoice.paid` (renewal) | `renew` `{days: 30, reset_traffic: true}` |
| `invoice.payment_failed` | `suspend` |
| `customer.subscription.deleted` | `cancel` |

Pass the Stripe event id through as `event_id` and you inherit Stripe's
delivery retries safely.

WooCommerce, Paddle, and crypto gateways follow the same pattern: map their
order/subscription id to `external_id` and their lifecycle hooks to actions.
