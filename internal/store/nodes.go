package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/ImErdis/xray-api/internal/domain"
)

const nodeCols = `id, name, api_address, api_port, api_tls, api_tls_server_name,
	api_tls_insecure, api_ca_cert, api_client_cert, api_client_key,
	status, last_seen_at, last_error, created_at, updated_at`

func scanNode(row interface{ Scan(...any) error }) (*domain.Node, error) {
	var n domain.Node
	var lastSeen sql.NullTime
	err := row.Scan(&n.ID, &n.Name, &n.APIAddress, &n.APIPort, &n.APITLS,
		&n.APITLSServerName, &n.APITLSInsecure, &n.APICACert, &n.APIClientCert,
		&n.APIClientKey, &n.Status, &lastSeen,
		&n.LastError, &n.CreatedAt, &n.UpdatedAt)
	if err != nil {
		return nil, mapErr(err)
	}
	if lastSeen.Valid {
		n.LastSeenAt = &lastSeen.Time
	}
	n.APIHasClientKey = n.APIClientKey != ""
	return &n, nil
}

func (s *Store) CreateNode(ctx context.Context, n *domain.Node) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO nodes (id, name, api_address, api_port, api_tls,
			api_tls_server_name, api_tls_insecure, api_ca_cert, api_client_cert,
			api_client_key, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		n.ID, n.Name, n.APIAddress, n.APIPort, n.APITLS,
		n.APITLSServerName, n.APITLSInsecure, n.APICACert, n.APIClientCert,
		n.APIClientKey, n.Status)
	return mapErr(err)
}

func (s *Store) GetNode(ctx context.Context, id string) (*domain.Node, error) {
	return scanNode(s.db.QueryRowContext(ctx,
		`SELECT `+nodeCols+` FROM nodes WHERE id = $1`, id))
}

func (s *Store) ListNodes(ctx context.Context) ([]*domain.Node, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+nodeCols+` FROM nodes ORDER BY created_at`)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	var out []*domain.Node
	for rows.Next() {
		n, err := scanNode(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (s *Store) UpdateNode(ctx context.Context, n *domain.Node) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE nodes SET name=$2, api_address=$3, api_port=$4, api_tls=$5,
			api_tls_server_name=$6, api_tls_insecure=$7, api_ca_cert=$8,
			api_client_cert=$9, api_client_key=$10, status=$11, updated_at=now()
		WHERE id = $1`,
		n.ID, n.Name, n.APIAddress, n.APIPort, n.APITLS,
		n.APITLSServerName, n.APITLSInsecure, n.APICACert, n.APIClientCert,
		n.APIClientKey, n.Status)
	return requireRow(res, err)
}

// SetNodeHealth records the outcome of a health probe.
func (s *Store) SetNodeHealth(ctx context.Context, id string, status domain.NodeStatus, lastErr string, seenAt *time.Time) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE nodes SET status=$2, last_error=$3,
			last_seen_at = COALESCE($4, last_seen_at), updated_at=now()
		WHERE id = $1`, id, status, lastErr, seenAt)
	return requireRow(res, err)
}

func (s *Store) DeleteNode(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM nodes WHERE id = $1`, id)
	return requireRow(res, err)
}

func requireRow(res sql.Result, err error) error {
	if err != nil {
		return mapErr(err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}
