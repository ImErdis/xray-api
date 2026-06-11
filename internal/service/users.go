package service

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/ImErdis/xray-api/internal/domain"
	"github.com/ImErdis/xray-api/internal/store"
	"github.com/ImErdis/xray-api/internal/sublink"
	"github.com/ImErdis/xray-api/internal/worker"
)

// UserService holds subscription-user business logic and orchestrates node
// provisioning via the worker manager.
type UserService struct {
	store         *store.Store
	mgr           *worker.Manager
	publicBaseURL string
}

func NewUserService(st *store.Store, mgr *worker.Manager, publicBaseURL string) *UserService {
	return &UserService{
		store:         st,
		mgr:           mgr,
		publicBaseURL: strings.TrimRight(publicBaseURL, "/"),
	}
}

// CreateInput is the payload for provisioning a user.
type CreateInput struct {
	Email          string
	PlanID         *string
	InboundIDs     []string
	DataLimitBytes *int64     // overrides plan limit when set
	ExpiresAt      *time.Time // overrides plan duration when set
	Note           string
	ExternalID     *string // billing-system reference (unique when set)
}

func (s *UserService) Create(ctx context.Context, in CreateInput) (*domain.User, error) {
	if strings.TrimSpace(in.Email) == "" {
		return nil, domain.Validationf("email is required")
	}
	if len(in.InboundIDs) == 0 {
		return nil, domain.Validationf("at least one inbound_id is required")
	}

	limit := in.DataLimitBytes
	expires := in.ExpiresAt

	// Snapshot plan limits at creation time unless explicitly overridden.
	if in.PlanID != nil {
		plan, err := s.store.GetPlan(ctx, *in.PlanID)
		if err != nil {
			return nil, err
		}
		if limit == nil {
			limit = plan.DataLimitBytes
		}
		if expires == nil && plan.DurationDays != nil {
			t := time.Now().AddDate(0, 0, *plan.DurationDays)
			expires = &t
		}
	}

	// Validate the inbounds exist before writing anything.
	inbounds, err := s.store.ListInboundsByIDs(ctx, in.InboundIDs)
	if err != nil {
		return nil, err
	}
	if len(inbounds) != len(dedup(in.InboundIDs)) {
		return nil, domain.Validationf("one or more inbound_ids do not exist")
	}

	u := &domain.User{
		ID:             uuid.NewString(),
		Email:          in.Email,
		UUID:           uuid.NewString(),
		TrojanPassword: randToken(18),
		PlanID:         in.PlanID,
		Status:         domain.UserStatusActive,
		DataLimitBytes: limit,
		ExpiresAt:      expires,
		SubToken:       randToken(24),
		Note:           in.Note,
		ExternalID:     in.ExternalID,
	}

	err = s.store.WithTx(ctx, func(tx *sql.Tx) error {
		if err := s.store.CreateUserTx(ctx, tx, u); err != nil {
			return err
		}
		for _, ib := range inbounds {
			if err := s.store.AddAssignmentTx(ctx, tx, u.ID, ib.ID); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	s.mgr.ReconcileForUser(ctx, u.ID)
	return s.store.GetUser(ctx, u.ID)
}

func (s *UserService) Get(ctx context.Context, id string) (*domain.User, error) {
	return s.store.GetUser(ctx, id)
}

func (s *UserService) List(ctx context.Context, f store.UserFilter) ([]*domain.User, int, error) {
	return s.store.ListUsers(ctx, f)
}

// UpdateInput patches mutable user fields. Pointer fields left nil are
// unchanged; to clear expiry pass a zero-value time via ClearExpiry.
type UpdateInput struct {
	PlanID         *string
	DataLimitBytes *int64
	ExpiresAt      *time.Time
	ClearExpiry    bool
	Note           *string
	Status         *domain.UserStatus // admin: disable/enable
}

func (s *UserService) Update(ctx context.Context, id string, in UpdateInput) (*domain.User, error) {
	u, err := s.store.GetUser(ctx, id)
	if err != nil {
		return nil, err
	}
	if in.PlanID != nil {
		u.PlanID = in.PlanID
	}
	if in.DataLimitBytes != nil {
		u.DataLimitBytes = in.DataLimitBytes
	}
	if in.ClearExpiry {
		u.ExpiresAt = nil
	} else if in.ExpiresAt != nil {
		u.ExpiresAt = in.ExpiresAt
	}
	if in.Note != nil {
		u.Note = *in.Note
	}
	if in.Status != nil {
		if !in.Status.Valid() {
			return nil, domain.Validationf("invalid status %q (want active|suspended|expired|disabled)", *in.Status)
		}
		u.Status = *in.Status
	}
	// Reactivate an expired user whose expiry now lies in the future.
	if u.Status == domain.UserStatusExpired && u.ExpiresAt != nil && u.ExpiresAt.After(time.Now()) {
		u.Status = domain.UserStatusActive
	}
	if err := s.store.UpdateUser(ctx, u); err != nil {
		return nil, err
	}
	s.mgr.ReconcileForUser(ctx, u.ID)
	return s.store.GetUser(ctx, id)
}

// SetInbounds replaces a user's assignment set and reconciles affected nodes.
func (s *UserService) SetInbounds(ctx context.Context, id string, inboundIDs []string) (*domain.User, error) {
	if _, err := s.store.GetUser(ctx, id); err != nil {
		return nil, err
	}
	inbounds, err := s.store.ListInboundsByIDs(ctx, inboundIDs)
	if err != nil {
		return nil, err
	}
	if len(inbounds) != len(dedup(inboundIDs)) {
		return nil, domain.Validationf("one or more inbound_ids do not exist")
	}
	// Capture old node set so removed-from nodes also reconcile.
	oldTargets, _ := s.store.AssignmentTargetsForUser(ctx, id)

	err = s.store.WithTx(ctx, func(tx *sql.Tx) error {
		return s.store.SetAssignmentsForUserTx(ctx, tx, id, inboundIDs)
	})
	if err != nil {
		return nil, err
	}

	s.reconcileNodes(ctx, oldTargets, inbounds)
	return s.store.GetUser(ctx, id)
}

func (s *UserService) Suspend(ctx context.Context, id string) (*domain.User, error) {
	return s.setStatusAndReconcile(ctx, id, domain.UserStatusDisabled)
}

// Resume reactivates a manually-disabled or suspended user. It refuses while
// still over quota (the enforcer would immediately re-suspend); the caller
// should reset traffic or raise the limit first.
func (s *UserService) Resume(ctx context.Context, id string) (*domain.User, error) {
	u, err := s.store.GetUser(ctx, id)
	if err != nil {
		return nil, err
	}
	if u.OverQuota() {
		return nil, domain.Validationf("user is over data limit; reset traffic or raise the limit first")
	}
	if u.ExpiresAt != nil && !u.ExpiresAt.After(time.Now()) {
		return nil, domain.Validationf("user is expired; extend expiry first")
	}
	return s.setStatusAndReconcile(ctx, id, domain.UserStatusActive)
}

func (s *UserService) setStatusAndReconcile(ctx context.Context, id string, status domain.UserStatus) (*domain.User, error) {
	if err := s.store.SetUserStatus(ctx, id, status); err != nil {
		return nil, err
	}
	s.mgr.ReconcileForUser(ctx, id)
	return s.store.GetUser(ctx, id)
}

// ResetTraffic zeroes usage and reactivates a quota-suspended user.
func (s *UserService) ResetTraffic(ctx context.Context, id string) (*domain.User, error) {
	u, err := s.store.GetUser(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.store.ResetTraffic(ctx, id); err != nil {
		return nil, err
	}
	if u.Status == domain.UserStatusSuspended &&
		(u.ExpiresAt == nil || u.ExpiresAt.After(time.Now())) {
		if err := s.store.SetUserStatus(ctx, id, domain.UserStatusActive); err != nil {
			return nil, err
		}
	}
	s.mgr.ReconcileForUser(ctx, id)
	return s.store.GetUser(ctx, id)
}

// RenewInput controls a subscription renewal.
type RenewInput struct {
	// Days extends expiry by this many days, counted from the current expiry
	// when it lies in the future, otherwise from now (so early renewals stack
	// and late renewals don't grant retroactive time).
	Days int
	// ResetTraffic zeroes usage counters (typical for monthly plans).
	ResetTraffic bool
	// PlanID optionally switches the plan; the new plan's data limit is
	// re-snapshotted onto the user.
	PlanID *string
}

// Renew atomically extends a user's subscription and reactivates them. This is
// the endpoint billing systems should call on successful payment.
func (s *UserService) Renew(ctx context.Context, id string, in RenewInput) (*domain.User, error) {
	if in.Days <= 0 && !in.ResetTraffic && in.PlanID == nil {
		return nil, domain.Validationf("renew requires days > 0, reset_traffic, or plan_id")
	}
	u, err := s.store.GetUser(ctx, id)
	if err != nil {
		return nil, err
	}

	if in.PlanID != nil {
		plan, err := s.store.GetPlan(ctx, *in.PlanID)
		if err != nil {
			return nil, err
		}
		u.PlanID = in.PlanID
		u.DataLimitBytes = plan.DataLimitBytes
	}

	if in.Days > 0 {
		base := time.Now()
		if u.ExpiresAt != nil && u.ExpiresAt.After(base) {
			base = *u.ExpiresAt
		}
		t := base.AddDate(0, 0, in.Days)
		u.ExpiresAt = &t
	}

	if in.ResetTraffic {
		if err := s.store.ResetTraffic(ctx, id); err != nil {
			return nil, err
		}
		u.UsedUploadBytes, u.UsedDownloadBytes = 0, 0
	}

	// Reactivate unless the admin explicitly disabled the user, or they would
	// still be over quota / expired after this renewal.
	if u.Status != domain.UserStatusDisabled && !u.OverQuota() &&
		(u.ExpiresAt == nil || u.ExpiresAt.After(time.Now())) {
		u.Status = domain.UserStatusActive
	}

	if err := s.store.UpdateUser(ctx, u); err != nil {
		return nil, err
	}
	s.mgr.ReconcileForUser(ctx, u.ID)
	return s.store.GetUser(ctx, id)
}

// RotateSubToken issues a new subscription token (invalidates old links).
func (s *UserService) RotateSubToken(ctx context.Context, id string) (*domain.User, error) {
	u, err := s.store.GetUser(ctx, id)
	if err != nil {
		return nil, err
	}
	u.SubToken = randToken(24)
	if err := s.store.UpdateUser(ctx, u); err != nil {
		return nil, err
	}
	return s.store.GetUser(ctx, id)
}

func (s *UserService) Delete(ctx context.Context, id string) error {
	// Tombstone assignments and reconcile so nodes drop the user, then delete.
	targets, err := s.store.AssignmentTargetsForUser(ctx, id)
	if err != nil {
		return err
	}
	if err := s.store.MarkAssignmentsRemovingForUser(ctx, id); err != nil {
		return err
	}
	for _, nodeID := range distinctNodes(targets) {
		s.mgr.Reconcile(nodeID)
	}
	// ON DELETE CASCADE removes assignments; for offline nodes the runtime
	// state is wiped on the node's next restart, and DB-desired no longer
	// includes this user, so convergence is preserved.
	return s.store.DeleteUser(ctx, id)
}

// Usage returns aggregated traffic for a user.
func (s *UserService) Usage(ctx context.Context, id string, from, to time.Time, groupBy string) ([]store.UsageBucket, error) {
	if _, err := s.store.GetUser(ctx, id); err != nil {
		return nil, err
	}
	return s.store.UserUsage(ctx, id, from, to, groupBy)
}

// SubURL builds the public subscription URL for a user.
func (s *UserService) SubURL(u *domain.User) string {
	return s.publicBaseURL + "/sub/" + u.SubToken
}

// --- subscription rendering ---

// Subscription bundles the data the public sub endpoint needs.
type Subscription struct {
	User     *domain.User
	Inbounds []*domain.Inbound
}

func (s *UserService) SubscriptionByToken(ctx context.Context, token string) (*Subscription, error) {
	u, err := s.store.GetUserBySubToken(ctx, token)
	if err != nil {
		return nil, err
	}
	inbounds, err := s.store.ListInboundsForUser(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	return &Subscription{User: u, Inbounds: inbounds}, nil
}

// RenderV2Ray renders the base64 link bundle.
func (sub *Subscription) RenderV2Ray() (string, error) {
	return sublink.V2Ray(sub.User, sub.Inbounds)
}

// RenderClash renders a Clash.Meta YAML config.
func (sub *Subscription) RenderClash() ([]byte, error) {
	return sublink.Clash(sub.User, sub.Inbounds)
}

// UserInfoHeader builds the subscription-userinfo header value.
func (sub *Subscription) UserInfoHeader() string {
	return sublink.UserInfoHeader(sub.User)
}

// --- helpers ---

func (s *UserService) reconcileNodes(ctx context.Context, oldTargets []*store.DesiredAssignment, newInbounds []*domain.Inbound) {
	seen := make(map[string]struct{})
	trigger := func(nodeID string) {
		if _, ok := seen[nodeID]; ok {
			return
		}
		seen[nodeID] = struct{}{}
		s.mgr.Reconcile(nodeID)
	}
	for _, t := range oldTargets {
		trigger(t.NodeID)
	}
	for _, ib := range newInbounds {
		trigger(ib.NodeID)
	}
}

func distinctNodes(targets []*store.DesiredAssignment) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, t := range targets {
		if _, ok := seen[t.NodeID]; ok {
			continue
		}
		seen[t.NodeID] = struct{}{}
		out = append(out, t.NodeID)
	}
	return out
}

func dedup(ids []string) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
