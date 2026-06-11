package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ImErdis/xray-api/internal/domain"
	"github.com/ImErdis/xray-api/internal/store"
)

func TestUpdateRejectsInvalidStatus(t *testing.T) {
	_, users, _, inboundID := testBilling(t)
	ctx := context.Background()

	u, err := users.Create(ctx, CreateInput{
		Email:      "st-" + uuid.NewString() + "@x.io",
		InboundIDs: []string{inboundID},
	})
	if err != nil {
		t.Fatal(err)
	}

	bogus := domain.UserStatus("bogus")
	if _, err := users.Update(ctx, u.ID, UpdateInput{Status: &bogus}); !errors.Is(err, domain.ErrValidation) {
		t.Errorf("invalid status: err = %v, want ErrValidation", err)
	}
	got, err := users.Get(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.UserStatusActive {
		t.Errorf("status changed to %q despite validation failure", got.Status)
	}

	disabled := domain.UserStatusDisabled
	updated, err := users.Update(ctx, u.ID, UpdateInput{Status: &disabled})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != domain.UserStatusDisabled {
		t.Errorf("status = %q, want disabled", updated.Status)
	}
}

func TestQuotaAutoSuspendAndResetTraffic(t *testing.T) {
	_, users, st, inboundID := testBilling(t)
	ctx := context.Background()

	limit := int64(1000)
	u, err := users.Create(ctx, CreateInput{
		Email:          "q-" + uuid.NewString() + "@x.io",
		InboundIDs:     []string{inboundID},
		DataLimitBytes: &limit,
	})
	if err != nil {
		t.Fatal(err)
	}
	ib, err := st.GetInbound(ctx, inboundID)
	if err != nil {
		t.Fatal(err)
	}

	// First delta keeps the user under quota.
	over, err := st.ApplyTrafficDeltas(ctx, ib.NodeID,
		[]store.TrafficDelta{{UserID: u.ID, Uplink: 300, Downlink: 300}}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(over) != 0 {
		t.Fatalf("under-quota user reported over quota: %v", over)
	}

	// Second delta crosses the limit: reported once and flipped to suspended.
	over, err = st.ApplyTrafficDeltas(ctx, ib.NodeID,
		[]store.TrafficDelta{{UserID: u.ID, Uplink: 300, Downlink: 300}}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(over) != 1 || over[0] != u.ID {
		t.Fatalf("over-quota ids = %v, want [%s]", over, u.ID)
	}
	got, err := users.Get(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.UserStatusSuspended {
		t.Fatalf("status = %q, want suspended", got.Status)
	}
	if got.UsedTotalBytes() != 1200 {
		t.Errorf("used = %d, want 1200", got.UsedTotalBytes())
	}

	// Resume refuses while still over quota.
	if _, err := users.Resume(ctx, u.ID); !errors.Is(err, domain.ErrValidation) {
		t.Errorf("resume over quota: err = %v, want ErrValidation", err)
	}

	// ResetTraffic zeroes counters and reactivates.
	reset, err := users.ResetTraffic(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reset.Status != domain.UserStatusActive || reset.UsedTotalBytes() != 0 {
		t.Errorf("after reset: status %q used %d, want active 0", reset.Status, reset.UsedTotalBytes())
	}

	// Usage history survives the counter reset.
	buckets, err := users.Usage(ctx, u.ID, time.Now().Add(-time.Hour), time.Now().Add(time.Hour), "day")
	if err != nil {
		t.Fatal(err)
	}
	var total int64
	for _, b := range buckets {
		total += b.Uplink + b.Downlink
	}
	if total != 1200 {
		t.Errorf("usage history total = %d, want 1200", total)
	}
}

func TestRenewSwitchesPlanAndReactivates(t *testing.T) {
	_, users, st, inboundID := testBilling(t)
	ctx := context.Background()

	bigLimit := int64(5000)
	plans := NewPlanService(st)
	plan, err := plans.Create(ctx, PlanInput{
		Name: "plan-" + uuid.NewString(), DataLimitBytes: &bigLimit,
	})
	if err != nil {
		t.Fatal(err)
	}

	smallLimit := int64(100)
	u, err := users.Create(ctx, CreateInput{
		Email:          "rp-" + uuid.NewString() + "@x.io",
		InboundIDs:     []string{inboundID},
		DataLimitBytes: &smallLimit,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Push the user over the small limit so they get suspended.
	ib, err := st.GetInbound(ctx, inboundID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.ApplyTrafficDeltas(ctx, ib.NodeID,
		[]store.TrafficDelta{{UserID: u.ID, Uplink: 100, Downlink: 100}}, time.Now()); err != nil {
		t.Fatal(err)
	}

	// Renewing onto the bigger plan re-snapshots the limit and reactivates
	// without a traffic reset (200 used < 5000 limit).
	renewed, err := users.Renew(ctx, u.ID, RenewInput{Days: 30, PlanID: &plan.ID})
	if err != nil {
		t.Fatal(err)
	}
	if renewed.PlanID == nil || *renewed.PlanID != plan.ID {
		t.Errorf("plan_id = %v, want %s", renewed.PlanID, plan.ID)
	}
	if renewed.DataLimitBytes == nil || *renewed.DataLimitBytes != bigLimit {
		t.Errorf("data_limit = %v, want %d", renewed.DataLimitBytes, bigLimit)
	}
	if renewed.Status != domain.UserStatusActive {
		t.Errorf("status = %q, want active", renewed.Status)
	}
}
