package worker

import (
	"context"
	"time"

	"github.com/ImErdis/xray-api/internal/domain"
	"github.com/ImErdis/xray-api/internal/metrics"
	"github.com/ImErdis/xray-api/internal/store"
	"github.com/ImErdis/xray-api/internal/xray"
)

// reconcile converges the node to the DB-desired state: process removals first
// (tombstoned assignments and non-active users), then push additions. All
// AlterInbound calls are idempotent, so re-running is always safe.
func (a *nodeActor) reconcile(ctx context.Context) {
	c, err := a.ensureClient()
	if err != nil {
		a.log.Debug("reconcile skipped, node unreachable", "err", err)
		return
	}

	removals, err := a.mgr.store.RemovalsForNode(ctx, a.node.ID)
	if err != nil {
		a.log.Warn("reconcile: load removals", "err", err)
		return
	}
	for _, d := range removals {
		rctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		err := c.RemoveUser(rctx, d.InboundTag, d.Email)
		cancel()
		if err != nil {
			a.log.Warn("reconcile: remove user", "email", d.Email, "tag", d.InboundTag, "err", err)
			metrics.ReconcileErrors.WithLabelValues(a.node.Name).Inc()
			a.dropClient()
			return
		}
		// Tombstoned assignments are deleted; assignments kept because the user
		// is merely inactive (suspended/expired) stay so they reappear on resume.
		if d.SyncStatus == domain.SyncRemoving {
			if err := a.mgr.store.DeleteAssignment(ctx, d.UserID, d.InboundID); err != nil {
				a.log.Warn("reconcile: delete assignment", "err", err)
			}
		}
	}

	desired, err := a.mgr.store.DesiredForNode(ctx, a.node.ID)
	if err != nil {
		a.log.Warn("reconcile: load desired", "err", err)
		return
	}
	for _, d := range desired {
		actx, cancel := context.WithTimeout(ctx, 10*time.Second)
		err := c.AddUser(actx, d.InboundTag, accountFor(d))
		cancel()
		if err != nil {
			a.log.Warn("reconcile: add user", "email", d.Email, "tag", d.InboundTag, "err", err)
			metrics.ReconcileErrors.WithLabelValues(a.node.Name).Inc()
			_ = a.mgr.store.MarkAssignmentSynced(ctx, d.UserID, d.InboundID, domain.SyncFailed, err.Error())
			a.dropClient()
			return
		}
		if d.SyncStatus != domain.SyncApplied {
			_ = a.mgr.store.MarkAssignmentSynced(ctx, d.UserID, d.InboundID, domain.SyncApplied, "")
		}
	}
}

func accountFor(d *store.DesiredAssignment) xray.Account {
	return xray.Account{
		Protocol: string(d.Protocol),
		Email:    d.Email,
		UUID:     d.UUID,
		Password: d.TrojanPass,
		Flow:     d.Flow,
	}
}
