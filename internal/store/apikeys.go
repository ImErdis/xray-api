package store

import (
	"context"
	"database/sql"

	"github.com/ImErdis/xray-api/internal/domain"
)

func (s *Store) CreateAPIKey(ctx context.Context, k *domain.APIKey) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO api_keys (id, name, key_hash) VALUES ($1, $2, $3)`,
		k.ID, k.Name, k.KeyHash)
	return mapErr(err)
}

// FindAPIKeyByHash returns the active (non-revoked) key matching the hash and
// touches last_used_at. Returns ErrNotFound when no live key matches.
func (s *Store) FindAPIKeyByHash(ctx context.Context, hash string) (*domain.APIKey, error) {
	var k domain.APIKey
	var lastUsed, revoked sql.NullTime
	err := s.db.QueryRowContext(ctx, `
		UPDATE api_keys SET last_used_at = now()
		WHERE key_hash = $1 AND revoked_at IS NULL
		RETURNING id, name, key_hash, created_at, last_used_at, revoked_at`, hash).
		Scan(&k.ID, &k.Name, &k.KeyHash, &k.CreatedAt, &lastUsed, &revoked)
	if err != nil {
		return nil, mapErr(err)
	}
	if lastUsed.Valid {
		k.LastUsedAt = &lastUsed.Time
	}
	return &k, nil
}

func (s *Store) ListAPIKeys(ctx context.Context) ([]*domain.APIKey, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, key_hash, created_at, last_used_at, revoked_at
		FROM api_keys ORDER BY created_at`)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	var out []*domain.APIKey
	for rows.Next() {
		var k domain.APIKey
		var lastUsed, revoked sql.NullTime
		if err := rows.Scan(&k.ID, &k.Name, &k.KeyHash, &k.CreatedAt,
			&lastUsed, &revoked); err != nil {
			return nil, mapErr(err)
		}
		if lastUsed.Valid {
			k.LastUsedAt = &lastUsed.Time
		}
		if revoked.Valid {
			k.RevokedAt = &revoked.Time
		}
		out = append(out, &k)
	}
	return out, rows.Err()
}

// RevokeAPIKey marks a key revoked (kept for audit; not deleted).
func (s *Store) RevokeAPIKey(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE api_keys SET revoked_at = now() WHERE id = $1 AND revoked_at IS NULL`, id)
	return requireRow(res, err)
}

// CountAPIKeys reports the number of live keys (used to detect first-run).
func (s *Store) CountAPIKeys(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT count(*) FROM api_keys WHERE revoked_at IS NULL`).Scan(&n)
	return n, mapErr(err)
}
