package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/ImErdis/xray-api/internal/domain"
)

const userCols = `id, email, uuid, trojan_password, plan_id, status, data_limit_bytes,
	used_upload_bytes, used_download_bytes, expires_at, sub_token, note,
	created_at, updated_at`

func scanUser(row interface{ Scan(...any) error }) (*domain.User, error) {
	var u domain.User
	var planID sql.NullString
	var expires sql.NullTime
	err := row.Scan(&u.ID, &u.Email, &u.UUID, &u.TrojanPassword, &planID, &u.Status,
		&u.DataLimitBytes, &u.UsedUploadBytes, &u.UsedDownloadBytes, &expires,
		&u.SubToken, &u.Note, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, mapErr(err)
	}
	if planID.Valid {
		u.PlanID = &planID.String
	}
	if expires.Valid {
		u.ExpiresAt = &expires.Time
	}
	return &u, nil
}

// CreateUserTx inserts a user within an existing transaction.
func (s *Store) CreateUserTx(ctx context.Context, tx *sql.Tx, u *domain.User) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO users (id, email, uuid, trojan_password, plan_id, status,
			data_limit_bytes, expires_at, sub_token, note)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		u.ID, u.Email, u.UUID, u.TrojanPassword, u.PlanID, u.Status,
		u.DataLimitBytes, u.ExpiresAt, u.SubToken, u.Note)
	return mapErr(err)
}

func (s *Store) GetUser(ctx context.Context, id string) (*domain.User, error) {
	return scanUser(s.db.QueryRowContext(ctx,
		`SELECT `+userCols+` FROM users WHERE id = $1`, id))
}

func (s *Store) GetUserBySubToken(ctx context.Context, token string) (*domain.User, error) {
	return scanUser(s.db.QueryRowContext(ctx,
		`SELECT `+userCols+` FROM users WHERE sub_token = $1`, token))
}

// UserFilter is the query filter for ListUsers.
type UserFilter struct {
	Status string
	PlanID string
	Query  string // matches email/note substring
	Limit  int
	Offset int
}

func (s *Store) ListUsers(ctx context.Context, f UserFilter) ([]*domain.User, int, error) {
	var conds []string
	var args []any
	ph := func() string { return fmt.Sprintf("$%d", len(args)) }

	if f.Status != "" {
		args = append(args, f.Status)
		conds = append(conds, "status = "+ph())
	}
	if f.PlanID != "" {
		args = append(args, f.PlanID)
		conds = append(conds, "plan_id = "+ph())
	}
	if f.Query != "" {
		args = append(args, "%"+f.Query+"%")
		p := ph()
		conds = append(conds, "(email ILIKE "+p+" OR note ILIKE "+p+")")
	}
	clause := ""
	if len(conds) > 0 {
		clause = " WHERE " + strings.Join(conds, " AND ")
	}

	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM users`+clause, args...).
		Scan(&total); err != nil {
		return nil, 0, mapErr(err)
	}

	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	args = append(args, limit)
	limitPh := ph()
	args = append(args, f.Offset)
	offsetPh := ph()

	q := `SELECT ` + userCols + ` FROM users` + clause +
		` ORDER BY created_at DESC LIMIT ` + limitPh + ` OFFSET ` + offsetPh
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, 0, mapErr(err)
	}
	defer rows.Close()
	var out []*domain.User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, u)
	}
	return out, total, rows.Err()
}

// UpdateUser persists mutable fields (plan, status, limit, expiry, note, token).
func (s *Store) UpdateUser(ctx context.Context, u *domain.User) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE users SET plan_id=$2, status=$3, data_limit_bytes=$4,
			expires_at=$5, note=$6, sub_token=$7, updated_at=now()
		WHERE id = $1`,
		u.ID, u.PlanID, u.Status, u.DataLimitBytes, u.ExpiresAt, u.Note, u.SubToken)
	return requireRow(res, err)
}

// SetUserStatus flips status (used by enforcer/expiry sweeper).
func (s *Store) SetUserStatus(ctx context.Context, id string, status domain.UserStatus) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE users SET status=$2, updated_at=now() WHERE id=$1`, id, status)
	return requireRow(res, err)
}

// ResetTraffic zeroes usage counters.
func (s *Store) ResetTraffic(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE users SET used_upload_bytes=0, used_download_bytes=0, updated_at=now()
		WHERE id=$1`, id)
	return requireRow(res, err)
}

func (s *Store) DeleteUser(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, id)
	return requireRow(res, err)
}

// UserIDsByEmail maps emails to user ids (the collector keys traffic by email).
func (s *Store) UserIDsByEmail(ctx context.Context, emails []string) (map[string]string, error) {
	if len(emails) == 0 {
		return map[string]string{}, nil
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT email, id FROM users WHERE email = ANY($1)`, emails)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	out := make(map[string]string, len(emails))
	for rows.Next() {
		var email, id string
		if err := rows.Scan(&email, &id); err != nil {
			return nil, mapErr(err)
		}
		out[email] = id
	}
	return out, rows.Err()
}

// ExpireDueUsers flips active users past their expiry to 'expired' and returns
// their ids so the caller can remove them from nodes.
func (s *Store) ExpireDueUsers(ctx context.Context, now time.Time) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		UPDATE users SET status='expired', updated_at=now()
		WHERE status='active' AND expires_at IS NOT NULL AND expires_at <= $1
		RETURNING id`, now)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, mapErr(err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
