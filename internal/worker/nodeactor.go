package worker

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/ImErdis/xray-api/internal/domain"
	"github.com/ImErdis/xray-api/internal/metrics"
	"github.com/ImErdis/xray-api/internal/xray"
)

const healthFailThreshold = 3

// nodeActor owns all gRPC interaction with a single node. Every mutation runs
// in this one goroutine, so reconcile/stats/health never interleave and races
// (e.g. suspend vs. reconcile) cannot occur. Commands re-read desired state
// from the DB at execution time, so stale triggers are harmless.
type nodeActor struct {
	mgr  *Manager
	node *domain.Node
	log  *slog.Logger

	reconcileCh chan struct{} // coalescing trigger, buffered size 1
	stopCh      chan struct{}
	stopOnce    sync.Once

	client    xray.Client
	failCount int
	online    bool
	// lastUptime is the node's uptime at the previous successful ping. A drop
	// means Xray restarted (runtime users wiped) even if no probe ever failed,
	// so the actor reconciles immediately instead of waiting for the periodic
	// pass.
	lastUptime uint32

	// pendingDeltas holds traffic that was read+reset from the node but not yet
	// persisted (DB write failed). It is merged into the next collection so a
	// transient DB error does not lose a reset interval's bytes.
	pendingDeltas map[string]*xray.UserTraffic
}

func newNodeActor(m *Manager, n *domain.Node) *nodeActor {
	return &nodeActor{
		mgr:           m,
		node:          n,
		log:           m.log.With("node", n.ID, "node_name", n.Name),
		reconcileCh:   make(chan struct{}, 1),
		stopCh:        make(chan struct{}),
		pendingDeltas: make(map[string]*xray.UserTraffic),
	}
}

func (a *nodeActor) triggerReconcile() {
	select {
	case a.reconcileCh <- struct{}{}:
	default: // already pending
	}
}

func (a *nodeActor) stop() { a.stopOnce.Do(func() { close(a.stopCh) }) }

func (a *nodeActor) close() {
	if a.client != nil {
		_ = a.client.Close()
		a.client = nil
	}
}

func (a *nodeActor) run(ctx context.Context) {
	defer a.close()
	a.log.Info("node actor started")

	health := time.NewTicker(a.mgr.cfg.HealthInterval)
	stats := time.NewTicker(a.mgr.cfg.StatsInterval)
	reconcile := time.NewTicker(a.mgr.cfg.ReconcileInterval)
	defer health.Stop()
	defer stats.Stop()
	defer reconcile.Stop()

	a.checkHealth(ctx) // probe immediately on startup

	for {
		select {
		case <-ctx.Done():
			return
		case <-a.stopCh:
			return
		case <-health.C:
			a.checkHealth(ctx)
		case <-stats.C:
			a.collect(ctx)
		case <-reconcile.C:
			a.reconcile(ctx)
		case <-a.reconcileCh:
			a.reconcile(ctx)
		}
	}
}

// ensureClient lazily dials the node, reusing the connection across calls.
func (a *nodeActor) ensureClient() (xray.Client, error) {
	if a.client != nil {
		return a.client, nil
	}
	c, err := a.mgr.dial(a.node)
	if err != nil {
		return nil, err
	}
	a.client = c
	return c, nil
}

func (a *nodeActor) dropClient() {
	if a.client != nil {
		_ = a.client.Close()
		a.client = nil
	}
}

func (a *nodeActor) checkHealth(ctx context.Context) {
	var uptime uint32
	c, err := a.ensureClient()
	if err == nil {
		cctx, cancel := context.WithTimeout(ctx, 8*time.Second)
		uptime, err = c.Ping(cctx)
		cancel()
	}
	if err != nil {
		a.failCount++
		a.dropClient()
		if a.failCount >= healthFailThreshold && a.online {
			a.online = false
			a.setHealth(ctx, domain.NodeStatusOffline, err.Error(), nil)
			metrics.NodeUp.WithLabelValues(a.node.Name).Set(0)
			a.log.Warn("node went offline", "err", err)
		} else if a.failCount < healthFailThreshold {
			a.setHealth(ctx, a.node.Status, err.Error(), nil)
		}
		return
	}

	a.failCount = 0
	now := time.Now()
	wasOffline := !a.online
	restarted := a.lastUptime > 0 && uptime < a.lastUptime
	a.lastUptime = uptime
	a.online = true
	a.setHealth(ctx, domain.NodeStatusOnline, "", &now)
	metrics.NodeUp.WithLabelValues(a.node.Name).Set(1)
	if wasOffline || restarted {
		// Xray restarted (or was unreachable): runtime users are wiped on
		// restart, so converge from DB-desired state now. The uptime check
		// catches fast restarts that never miss a health probe.
		a.log.Info("node online, reconciling", "restart_detected", restarted)
		a.reconcile(ctx)
	}
}

func (a *nodeActor) setHealth(ctx context.Context, status domain.NodeStatus, errMsg string, seen *time.Time) {
	a.node.Status = status
	if err := a.mgr.store.SetNodeHealth(ctx, a.node.ID, status, errMsg, seen); err != nil {
		a.log.Warn("persist health", "err", err)
	}
}
