package store

import (
	"context"

	"github.com/ImErdis/xray-api/internal/domain"
)

const planCols = `id, name, data_limit_bytes, duration_days, created_at, updated_at`

func scanPlan(row interface{ Scan(...any) error }) (*domain.Plan, error) {
	var p domain.Plan
	err := row.Scan(&p.ID, &p.Name, &p.DataLimitBytes, &p.DurationDays,
		&p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, mapErr(err)
	}
	return &p, nil
}

func (s *Store) CreatePlan(ctx context.Context, p *domain.Plan) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO plans (id, name, data_limit_bytes, duration_days)
		VALUES ($1, $2, $3, $4)`,
		p.ID, p.Name, p.DataLimitBytes, p.DurationDays)
	return mapErr(err)
}

func (s *Store) GetPlan(ctx context.Context, id string) (*domain.Plan, error) {
	return scanPlan(s.db.QueryRowContext(ctx,
		`SELECT `+planCols+` FROM plans WHERE id = $1`, id))
}

func (s *Store) ListPlans(ctx context.Context) ([]*domain.Plan, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+planCols+` FROM plans ORDER BY created_at`)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	var out []*domain.Plan
	for rows.Next() {
		p, err := scanPlan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) UpdatePlan(ctx context.Context, p *domain.Plan) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE plans SET name=$2, data_limit_bytes=$3, duration_days=$4, updated_at=now()
		WHERE id = $1`,
		p.ID, p.Name, p.DataLimitBytes, p.DurationDays)
	return requireRow(res, err)
}

func (s *Store) DeletePlan(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM plans WHERE id = $1`, id)
	return requireRow(res, err)
}
