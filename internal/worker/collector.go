package worker

import (
	"context"
	"time"

	"github.com/ImErdis/xray-api/internal/metrics"
	"github.com/ImErdis/xray-api/internal/store"
	"github.com/ImErdis/xray-api/internal/xray"
)

// collect reads (and resets) per-user traffic from the node, merges any deltas
// left over from a previous failed DB write, persists them, and — when the DB
// reports users that crossed their quota — suspends them on every node.
func (a *nodeActor) collect(ctx context.Context) {
	c, err := a.ensureClient()
	if err != nil {
		return // offline; nothing to collect
	}

	qctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	traffic, err := c.QueryUserTraffic(qctx, true)
	cancel()
	if err != nil {
		a.log.Warn("collect: query traffic", "err", err)
		a.dropClient()
		return
	}

	// Merge freshly-read counters into any carried-over pending deltas.
	for i := range traffic {
		t := &traffic[i]
		if p, ok := a.pendingDeltas[t.Email]; ok {
			p.Uplink += t.Uplink
			p.Downlink += t.Downlink
		} else {
			cp := *t
			a.pendingDeltas[t.Email] = &cp
		}
	}
	if len(a.pendingDeltas) == 0 {
		return
	}

	// Resolve emails to user ids (stats are keyed by email).
	emails := make([]string, 0, len(a.pendingDeltas))
	for e := range a.pendingDeltas {
		emails = append(emails, e)
	}
	idByEmail, err := a.mgr.store.UserIDsByEmail(ctx, emails)
	if err != nil {
		a.log.Warn("collect: resolve emails", "err", err)
		return // keep pendingDeltas; retry next cycle
	}

	deltas := make([]store.TrafficDelta, 0, len(a.pendingDeltas))
	for email, t := range a.pendingDeltas {
		id, ok := idByEmail[email]
		if !ok {
			// Unknown email (user deleted, or hand-added on the node). Drop it.
			continue
		}
		deltas = append(deltas, store.TrafficDelta{
			UserID:   id,
			Uplink:   t.Uplink,
			Downlink: t.Downlink,
		})
	}

	overQuota, err := a.mgr.store.ApplyTrafficDeltas(ctx, a.node.ID, deltas, time.Now())
	if err != nil {
		a.log.Warn("collect: persist deltas", "err", err)
		return // DB write failed; keep pendingDeltas, merge next cycle
	}
	for _, d := range deltas {
		metrics.TrafficCollected.WithLabelValues(a.node.Name, "uplink").Add(float64(d.Uplink))
		metrics.TrafficCollected.WithLabelValues(a.node.Name, "downlink").Add(float64(d.Downlink))
	}
	// Success: clear the buffer.
	a.pendingDeltas = make(map[string]*xray.UserTraffic)

	// Suspended over-quota users must be removed from every node they're on.
	for _, userID := range overQuota {
		metrics.UsersSuspended.Inc()
		a.log.Info("user suspended over quota", "user", userID)
		a.mgr.ReconcileForUser(ctx, userID)
	}
}
