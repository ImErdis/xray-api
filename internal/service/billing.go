package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/ImErdis/xray-api/internal/domain"
	"github.com/ImErdis/xray-api/internal/store"
)

// BillingService processes provider-agnostic billing webhooks. Any billing
// system (Stripe, WooCommerce, a crypto gateway) that can POST signed JSON can
// drive the full subscription lifecycle through one endpoint.
type BillingService struct {
	store *store.Store
	users *UserService
}

func NewBillingService(st *store.Store, users *UserService) *BillingService {
	return &BillingService{store: st, users: users}
}

// VerifySignature checks the hex HMAC-SHA256 of the raw request body against
// the shared webhook secret, in constant time.
func VerifySignature(secret string, body []byte, signature string) bool {
	if secret == "" || signature == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}

// BillingEvent is the webhook payload. The user is addressed by external_id
// (preferred — set at provision time) or email. See docs/billing-webhooks.md.
type BillingEvent struct {
	// EventID is the provider's unique event id; used for replay-safe
	// idempotency. Required.
	EventID string `json:"event_id"`
	// Action is one of provision|renew|suspend|resume|cancel.
	Action string `json:"action"`

	// User reference for renew/suspend/resume/cancel.
	ExternalID string `json:"external_id,omitempty"`
	Email      string `json:"email,omitempty"`

	// Provision payload (action=provision).
	Provision *ProvisionPayload `json:"provision,omitempty"`
	// Renew payload (action=renew).
	Renew *RenewPayload `json:"renew,omitempty"`
}

type ProvisionPayload struct {
	Email          string     `json:"email"`
	PlanID         *string    `json:"plan_id,omitempty"`
	InboundIDs     []string   `json:"inbound_ids"`
	DataLimitBytes *int64     `json:"data_limit_bytes,omitempty"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
	Note           string     `json:"note,omitempty"`
}

type RenewPayload struct {
	Days         int     `json:"days"`
	ResetTraffic bool    `json:"reset_traffic"`
	PlanID       *string `json:"plan_id,omitempty"`
}

// BillingResult reports what a webhook delivery did.
type BillingResult struct {
	Duplicate bool         `json:"duplicate"`
	Action    string       `json:"action"`
	User      *domain.User `json:"user,omitempty"`
}

// Process validates, deduplicates, and executes a billing event.
func (s *BillingService) Process(ctx context.Context, raw []byte) (*BillingResult, error) {
	var ev BillingEvent
	if err := json.Unmarshal(raw, &ev); err != nil {
		return nil, domain.Validationf("invalid JSON: " + err.Error())
	}
	if ev.EventID == "" {
		return nil, domain.Validationf("event_id is required")
	}
	switch ev.Action {
	case "provision", "renew", "suspend", "resume", "cancel":
	default:
		return nil, domain.Validationf("action must be provision|renew|suspend|resume|cancel")
	}

	fresh, err := s.store.RecordWebhookEvent(ctx, ev.EventID, ev.Action)
	if err != nil {
		return nil, err
	}
	if !fresh {
		// Replay or provider retry of an event we already handled.
		return &BillingResult{Duplicate: true, Action: ev.Action}, nil
	}

	res := &BillingResult{Action: ev.Action}
	switch ev.Action {
	case "provision":
		res.User, err = s.provision(ctx, &ev)
	case "renew":
		res.User, err = s.renew(ctx, &ev)
	case "suspend":
		res.User, err = s.withUser(ctx, &ev, s.users.Suspend)
	case "resume":
		res.User, err = s.withUser(ctx, &ev, s.users.Resume)
	case "cancel":
		err = s.cancel(ctx, &ev)
	}
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (s *BillingService) provision(ctx context.Context, ev *BillingEvent) (*domain.User, error) {
	if ev.Provision == nil {
		return nil, domain.Validationf("provision payload is required")
	}
	if ev.ExternalID == "" {
		return nil, domain.Validationf("external_id is required for provision")
	}
	// Idempotent against billing-side retries that carry fresh event ids:
	// an existing user with this external_id is returned as-is.
	if u, err := s.store.GetUserByExternalID(ctx, ev.ExternalID); err == nil {
		return u, nil
	} else if !errors.Is(err, domain.ErrNotFound) {
		return nil, err
	}
	p := ev.Provision
	return s.users.Create(ctx, CreateInput{
		Email:          p.Email,
		PlanID:         p.PlanID,
		InboundIDs:     p.InboundIDs,
		DataLimitBytes: p.DataLimitBytes,
		ExpiresAt:      p.ExpiresAt,
		Note:           p.Note,
		ExternalID:     &ev.ExternalID,
	})
}

func (s *BillingService) renew(ctx context.Context, ev *BillingEvent) (*domain.User, error) {
	if ev.Renew == nil {
		return nil, domain.Validationf("renew payload is required")
	}
	u, err := s.resolveUser(ctx, ev)
	if err != nil {
		return nil, err
	}
	return s.users.Renew(ctx, u.ID, RenewInput{
		Days:         ev.Renew.Days,
		ResetTraffic: ev.Renew.ResetTraffic,
		PlanID:       ev.Renew.PlanID,
	})
}

// cancel disables the user (removed from all nodes) but keeps the record and
// stats, so a re-subscribe can resume rather than re-provision.
func (s *BillingService) cancel(ctx context.Context, ev *BillingEvent) error {
	u, err := s.resolveUser(ctx, ev)
	if err != nil {
		return err
	}
	_, err = s.users.Suspend(ctx, u.ID)
	return err
}

func (s *BillingService) withUser(ctx context.Context, ev *BillingEvent,
	fn func(ctx context.Context, id string) (*domain.User, error)) (*domain.User, error) {
	u, err := s.resolveUser(ctx, ev)
	if err != nil {
		return nil, err
	}
	return fn(ctx, u.ID)
}

func (s *BillingService) resolveUser(ctx context.Context, ev *BillingEvent) (*domain.User, error) {
	switch {
	case ev.ExternalID != "":
		u, err := s.store.GetUserByExternalID(ctx, ev.ExternalID)
		if err != nil {
			return nil, fmt.Errorf("user with external_id %q: %w", ev.ExternalID, err)
		}
		return u, nil
	case ev.Email != "":
		u, err := s.store.GetUserByEmail(ctx, ev.Email)
		if err != nil {
			return nil, fmt.Errorf("user with email %q: %w", ev.Email, err)
		}
		return u, nil
	default:
		return nil, domain.Validationf("external_id or email is required")
	}
}
