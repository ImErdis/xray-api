package service

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ImErdis/xray-api/internal/domain"
	"github.com/ImErdis/xray-api/internal/store"
	"github.com/ImErdis/xray-api/internal/worker"
	"github.com/ImErdis/xray-api/internal/xray"
)

// fakeDial keeps worker.Manager constructible without a reachable node.
func fakeDial(*domain.Node) (xray.Client, error) {
	return nil, context.DeadlineExceeded
}

func testBilling(t *testing.T) (*BillingService, *UserService, *store.Store, string) {
	t.Helper()
	dsn := os.Getenv("TEST_PG_DSN")
	if dsn == "" {
		t.Skip("TEST_PG_DSN not set; skipping Postgres integration test")
	}
	ctx := context.Background()
	st, err := store.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	mgr := worker.NewManager(st, fakeDial, worker.Config{
		HealthInterval: time.Minute, StatsInterval: time.Minute, ReconcileInterval: time.Hour,
	}, slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})))

	users := NewUserService(st, mgr, "http://test")
	billing := NewBillingService(st, users)

	// One node + inbound to provision against.
	node := &domain.Node{ID: uuid.NewString(), Name: "bt-" + uuid.NewString(),
		APIAddress: "x", APIPort: 1, Status: domain.NodeStatusUnknown}
	if err := st.CreateNode(ctx, node); err != nil {
		t.Fatal(err)
	}
	ib := &domain.Inbound{ID: uuid.NewString(), NodeID: node.ID, Tag: "t",
		Protocol: domain.ProtocolVLESS, PublicHost: "h", PublicPort: 443,
		Network: domain.NetworkTCP, Security: domain.SecurityNone}
	if err := st.CreateInbound(ctx, ib); err != nil {
		t.Fatal(err)
	}
	return billing, users, st, ib.ID
}

func TestBillingProvisionRenewLifecycle(t *testing.T) {
	billing, _, _, inboundID := testBilling(t)
	ctx := context.Background()

	extID := "sub_" + uuid.NewString()
	email := "bill-" + uuid.NewString() + "@x.io"
	provision := func(eventID string) *BillingResult {
		t.Helper()
		raw, _ := json.Marshal(BillingEvent{
			EventID:    eventID,
			Action:     "provision",
			ExternalID: extID,
			Provision: &ProvisionPayload{
				Email:      email,
				InboundIDs: []string{inboundID},
			},
		})
		res, err := billing.Process(ctx, raw)
		if err != nil {
			t.Fatalf("provision: %v", err)
		}
		return res
	}

	evt1 := "evt_" + uuid.NewString()
	res := provision(evt1)
	if res.Duplicate || res.User == nil || res.User.ExternalID == nil || *res.User.ExternalID != extID {
		t.Fatalf("unexpected provision result: %+v", res)
	}
	userID := res.User.ID

	// Same event id replayed → duplicate, nothing executed.
	if res := provision(evt1); !res.Duplicate {
		t.Error("replayed event id should report duplicate")
	}

	// New event id, same external_id (provider retry with fresh id) → same user.
	if res := provision("evt_" + uuid.NewString()); res.Duplicate || res.User.ID != userID {
		t.Errorf("re-provision should be idempotent on external_id, got %+v", res)
	}

	// Renew by external_id: +30 days and traffic reset.
	raw, _ := json.Marshal(BillingEvent{
		EventID:    "evt_" + uuid.NewString(),
		Action:     "renew",
		ExternalID: extID,
		Renew:      &RenewPayload{Days: 30, ResetTraffic: true},
	})
	renewed, err := billing.Process(ctx, raw)
	if err != nil {
		t.Fatalf("renew: %v", err)
	}
	if renewed.User.ExpiresAt == nil {
		t.Fatal("renew did not set expiry")
	}
	days := time.Until(*renewed.User.ExpiresAt).Hours() / 24
	if days < 29 || days > 31 {
		t.Errorf("expiry ~30d out expected, got %.1f days", days)
	}
	if renewed.User.Status != domain.UserStatusActive {
		t.Errorf("renewed user should be active, got %s", renewed.User.Status)
	}

	// Cancel disables the user.
	raw, _ = json.Marshal(BillingEvent{
		EventID: "evt_" + uuid.NewString(), Action: "cancel", ExternalID: extID,
	})
	if _, err := billing.Process(ctx, raw); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	final, err := billing.users.Get(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	if final.Status != domain.UserStatusDisabled {
		t.Errorf("cancelled user should be disabled, got %s", final.Status)
	}
}

func TestRenewStacksFromFutureExpiry(t *testing.T) {
	_, users, st, inboundID := testBilling(t)
	ctx := context.Background()

	future := time.Now().AddDate(0, 0, 10).Truncate(time.Second)
	u, err := users.Create(ctx, CreateInput{
		Email:      "stack-" + uuid.NewString() + "@x.io",
		InboundIDs: []string{inboundID},
		ExpiresAt:  &future,
	})
	if err != nil {
		t.Fatal(err)
	}

	renewed, err := users.Renew(ctx, u.ID, RenewInput{Days: 30})
	if err != nil {
		t.Fatal(err)
	}
	want := future.AddDate(0, 0, 30)
	if got := *renewed.ExpiresAt; got.Sub(want).Abs() > time.Minute {
		t.Errorf("early renewal should stack: got %v want ~%v", got, want)
	}

	// Expired user: renewal counts from now, not from the old expiry.
	past := time.Now().AddDate(0, 0, -20)
	if _, err := users.Update(ctx, u.ID, UpdateInput{ExpiresAt: &past}); err != nil {
		t.Fatal(err)
	}
	_ = st // (store only needed for setup elsewhere)
	renewed, err = users.Renew(ctx, u.ID, RenewInput{Days: 30})
	if err != nil {
		t.Fatal(err)
	}
	days := time.Until(*renewed.ExpiresAt).Hours() / 24
	if days < 29 || days > 31 {
		t.Errorf("late renewal should count from now: got %.1f days out", days)
	}
	if renewed.Status != domain.UserStatusActive {
		t.Errorf("late renewal should reactivate, got %s", renewed.Status)
	}
}
