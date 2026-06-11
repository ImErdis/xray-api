// Package worker runs the background control loops that converge each Xray
// node to the desired state held in the database: per-node reconcile/health/
// stats actors plus global expiry and snapshot-pruning sweepers.
package worker

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/ImErdis/xray-api/internal/domain"
	"github.com/ImErdis/xray-api/internal/store"
	"github.com/ImErdis/xray-api/internal/xray"
)

// Config tunes the worker loops.
type Config struct {
	HealthInterval        time.Duration
	StatsInterval         time.Duration
	ReconcileInterval     time.Duration
	SnapshotRetentionDays int
}

// DialFunc opens a gRPC client for a node (injected for testability).
type DialFunc func(n *domain.Node) (xray.Client, error)

// DefaultDial dials a node using its stored connection settings.
func DefaultDial(n *domain.Node) (xray.Client, error) {
	return xray.Dial(xray.DialOptions{
		Address:       n.APIAddress,
		Port:          n.APIPort,
		TLS:           n.APITLS,
		TLSServerName: n.APITLSServerName,
		TLSInsecure:   n.APITLSInsecure,
		CACertPEM:     n.APICACert,
		ClientCertPEM: n.APIClientCert,
		ClientKeyPEM:  n.APIClientKey,
		DialTimeout:   10 * time.Second,
	})
}

// Manager owns the lifecycle of all node actors and the global sweepers.
type Manager struct {
	store *store.Store
	dial  DialFunc
	cfg   Config
	log   *slog.Logger

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu     sync.Mutex
	actors map[string]*nodeActor
}

func NewManager(st *store.Store, dial DialFunc, cfg Config, log *slog.Logger) *Manager {
	if dial == nil {
		dial = DefaultDial
	}
	return &Manager{
		store:  st,
		dial:   dial,
		cfg:    cfg,
		log:    log,
		actors: make(map[string]*nodeActor),
	}
}

// Start spawns actors for all existing nodes and the global sweepers. It
// returns immediately; loops run until Stop or ctx cancellation.
func (m *Manager) Start(ctx context.Context) error {
	m.ctx, m.cancel = context.WithCancel(ctx)

	nodes, err := m.store.ListNodes(m.ctx)
	if err != nil {
		return err
	}
	for _, n := range nodes {
		if n.Status != domain.NodeStatusDisabled {
			m.StartNode(n)
		}
	}

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		m.runExpirySweeper(m.ctx)
	}()
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		m.runSnapshotPruner(m.ctx)
	}()
	return nil
}

// Stop signals all loops and waits for them to exit.
func (m *Manager) Stop() {
	if m.cancel != nil {
		m.cancel()
	}
	m.wg.Wait()
	m.mu.Lock()
	for _, a := range m.actors {
		a.close()
	}
	m.mu.Unlock()
}

// StartNode spawns (or restarts) an actor for a node.
func (m *Manager) StartNode(n *domain.Node) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.actors[n.ID]; ok {
		existing.stop()
		delete(m.actors, n.ID)
	}
	a := newNodeActor(m, n)
	m.actors[n.ID] = a
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		a.run(m.ctx)
	}()
}

// StopNode tears down a node's actor (on delete/disable).
func (m *Manager) StopNode(nodeID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if a, ok := m.actors[nodeID]; ok {
		a.stop()
		delete(m.actors, nodeID)
	}
}

// Reconcile triggers an immediate reconcile pass for one node (no-op if the
// node has no actor).
func (m *Manager) Reconcile(nodeID string) {
	m.mu.Lock()
	a := m.actors[nodeID]
	m.mu.Unlock()
	if a != nil {
		a.triggerReconcile()
	}
}

// ReconcileForUser triggers reconcile on every node the user is assigned to.
// Used after suspend/resume/expire/delete and assignment changes so the change
// propagates promptly instead of waiting for the periodic pass.
func (m *Manager) ReconcileForUser(ctx context.Context, userID string) {
	targets, err := m.store.AssignmentTargetsForUser(ctx, userID)
	if err != nil {
		m.log.Warn("reconcile for user: list targets", "user", userID, "err", err)
		return
	}
	seen := make(map[string]struct{})
	for _, t := range targets {
		if _, ok := seen[t.NodeID]; ok {
			continue
		}
		seen[t.NodeID] = struct{}{}
		m.Reconcile(t.NodeID)
	}
}
