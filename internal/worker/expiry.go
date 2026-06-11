package worker

import (
	"context"
	"time"

	"github.com/ImErdis/xray-api/internal/metrics"
)

const expirySweepInterval = 60 * time.Second

// runExpirySweeper periodically marks due users expired and triggers their
// removal from nodes. Idempotent: reconcilers also exclude non-active users,
// so a missed trigger still converges on the next pass.
func (m *Manager) runExpirySweeper(ctx context.Context) {
	t := time.NewTicker(expirySweepInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			ids, err := m.store.ExpireDueUsers(ctx, time.Now())
			if err != nil {
				m.log.Warn("expiry sweep", "err", err)
				continue
			}
			for _, id := range ids {
				metrics.UsersExpired.Inc()
				m.log.Info("user expired", "user", id)
				m.ReconcileForUser(ctx, id)
			}
		}
	}
}

// runSnapshotPruner deletes traffic snapshots past the retention window.
func (m *Manager) runSnapshotPruner(ctx context.Context) {
	if m.cfg.SnapshotRetentionDays <= 0 {
		return // retention disabled
	}
	t := time.NewTicker(6 * time.Hour)
	defer t.Stop()
	prune := func() {
		cutoff := time.Now().AddDate(0, 0, -m.cfg.SnapshotRetentionDays)
		n, err := m.store.PruneSnapshots(ctx, cutoff)
		if err != nil {
			m.log.Warn("snapshot prune", "err", err)
			return
		}
		if n > 0 {
			m.log.Info("pruned old snapshots", "rows", n)
		}
	}
	prune()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			prune()
		}
	}
}
