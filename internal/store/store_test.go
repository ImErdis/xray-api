package store

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ImErdis/xray-api/internal/domain"
)

// testStore opens a store against TEST_PG_DSN, skipping if unset. Each test run
// shares the schema (migrations are idempotent); tests use unique names/emails
// to avoid collisions.
func testStore(t *testing.T) *Store {
	t.Helper()
	dsn := os.Getenv("TEST_PG_DSN")
	if dsn == "" {
		t.Skip("TEST_PG_DSN not set; skipping Postgres integration test")
	}
	st, err := Open(context.Background(), dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestNodeAndInboundCRUD(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()

	n := &domain.Node{
		ID: uuid.NewString(), Name: "node-" + uuid.NewString(),
		APIAddress: "10.0.0.1", APIPort: 10085, Status: domain.NodeStatusUnknown,
	}
	if err := st.CreateNode(ctx, n); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetNode(ctx, n.ID)
	if err != nil || got.Name != n.Name {
		t.Fatalf("get node: %v / %+v", err, got)
	}

	ib := &domain.Inbound{
		ID: uuid.NewString(), NodeID: n.ID, Tag: "vless-ws",
		Protocol: domain.ProtocolVLESS, PublicHost: "h", PublicPort: 443,
		Network: domain.NetworkWS, Security: domain.SecurityTLS,
	}
	if err := st.CreateInbound(ctx, ib); err != nil {
		t.Fatal(err)
	}
	ibs, err := st.ListInboundsByNode(ctx, n.ID)
	if err != nil || len(ibs) != 1 {
		t.Fatalf("list inbounds: %v / %d", err, len(ibs))
	}

	// Cascade: deleting the node removes its inbounds.
	if err := st.DeleteNode(ctx, n.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetInbound(ctx, ib.ID); err != domain.ErrNotFound {
		t.Fatalf("expected inbound gone after node delete, got %v", err)
	}
}

func TestQuotaSuspensionViaTrafficDeltas(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()

	limit := int64(1000)
	u := &domain.User{
		ID: uuid.NewString(), Email: "u-" + uuid.NewString() + "@x.io",
		UUID: uuid.NewString(), TrojanPassword: uuid.NewString(),
		Status: domain.UserStatusActive, DataLimitBytes: &limit,
		SubToken: uuid.NewString(),
	}
	if err := st.WithTx(ctx, func(tx *sql.Tx) error {
		return st.CreateUserTx(ctx, tx, u)
	}); err != nil {
		t.Fatal(err)
	}

	node := &domain.Node{ID: uuid.NewString(), Name: "n-" + uuid.NewString(),
		APIAddress: "x", APIPort: 1, Status: domain.NodeStatusUnknown}
	if err := st.CreateNode(ctx, node); err != nil {
		t.Fatal(err)
	}

	// Under-limit delta: stays active.
	over, err := st.ApplyTrafficDeltas(ctx, node.ID,
		[]TrafficDelta{{UserID: u.ID, Uplink: 400, Downlink: 400}}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(over) != 0 {
		t.Fatalf("did not expect suspension yet, got %v", over)
	}

	// Crossing the limit: returned as over-quota and flipped to suspended.
	over, err = st.ApplyTrafficDeltas(ctx, node.ID,
		[]TrafficDelta{{UserID: u.ID, Uplink: 100, Downlink: 200}}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(over) != 1 || over[0] != u.ID {
		t.Fatalf("expected user suspended, got %v", over)
	}
	got, _ := st.GetUser(ctx, u.ID)
	if got.Status != domain.UserStatusSuspended {
		t.Fatalf("status = %s, want suspended", got.Status)
	}
	if got.UsedTotalBytes() != 1100 {
		t.Fatalf("used = %d, want 1100", got.UsedTotalBytes())
	}
}
