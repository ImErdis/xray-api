package service

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"github.com/ImErdis/xray-api/internal/domain"
	"github.com/ImErdis/xray-api/internal/store"
)

// PlanService holds plan business logic.
type PlanService struct {
	store *store.Store
}

func NewPlanService(st *store.Store) *PlanService { return &PlanService{store: st} }

// PlanInput is the create/update payload for a plan.
type PlanInput struct {
	Name           string
	DataLimitBytes *int64
	DurationDays   *int
}

func (s *PlanService) Create(ctx context.Context, in PlanInput) (*domain.Plan, error) {
	if strings.TrimSpace(in.Name) == "" {
		return nil, domain.Validationf("name is required")
	}
	if in.DataLimitBytes != nil && *in.DataLimitBytes < 0 {
		return nil, domain.Validationf("data_limit_bytes must be >= 0")
	}
	if in.DurationDays != nil && *in.DurationDays < 0 {
		return nil, domain.Validationf("duration_days must be >= 0")
	}
	p := &domain.Plan{
		ID:             uuid.NewString(),
		Name:           in.Name,
		DataLimitBytes: in.DataLimitBytes,
		DurationDays:   in.DurationDays,
	}
	if err := s.store.CreatePlan(ctx, p); err != nil {
		return nil, err
	}
	return s.store.GetPlan(ctx, p.ID)
}

func (s *PlanService) Get(ctx context.Context, id string) (*domain.Plan, error) {
	return s.store.GetPlan(ctx, id)
}

func (s *PlanService) List(ctx context.Context) ([]*domain.Plan, error) {
	return s.store.ListPlans(ctx)
}

func (s *PlanService) Update(ctx context.Context, id string, in PlanInput) (*domain.Plan, error) {
	p, err := s.store.GetPlan(ctx, id)
	if err != nil {
		return nil, err
	}
	if in.Name != "" {
		p.Name = in.Name
	}
	p.DataLimitBytes = in.DataLimitBytes
	p.DurationDays = in.DurationDays
	if err := s.store.UpdatePlan(ctx, p); err != nil {
		return nil, err
	}
	return s.store.GetPlan(ctx, id)
}

func (s *PlanService) Delete(ctx context.Context, id string) error {
	return s.store.DeletePlan(ctx, id)
}
