package store

import (
	"context"

	"github.com/ImErdis/xray-api/internal/domain"
)

const inboundCols = `id, node_id, tag, protocol, listen_port, public_host, public_port,
	network, security, ws_path, host_header, grpc_service_name, sni, fingerprint,
	reality_public_key, reality_short_id, flow, method, xhttp_mode, remark,
	created_at, updated_at`

func scanInbound(row interface{ Scan(...any) error }) (*domain.Inbound, error) {
	var ib domain.Inbound
	err := row.Scan(&ib.ID, &ib.NodeID, &ib.Tag, &ib.Protocol, &ib.ListenPort,
		&ib.PublicHost, &ib.PublicPort, &ib.Network, &ib.Security, &ib.WSPath,
		&ib.HostHeader, &ib.GRPCServiceName, &ib.SNI, &ib.Fingerprint,
		&ib.RealityPublicKey, &ib.RealityShortID, &ib.Flow, &ib.Method, &ib.XHTTPMode,
		&ib.Remark, &ib.CreatedAt, &ib.UpdatedAt)
	if err != nil {
		return nil, mapErr(err)
	}
	return &ib, nil
}

func (s *Store) CreateInbound(ctx context.Context, ib *domain.Inbound) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO inbounds (id, node_id, tag, protocol, listen_port, public_host,
			public_port, network, security, ws_path, host_header, grpc_service_name,
			sni, fingerprint, reality_public_key, reality_short_id, flow, method,
			xhttp_mode, remark)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)`,
		ib.ID, ib.NodeID, ib.Tag, ib.Protocol, ib.ListenPort, ib.PublicHost,
		ib.PublicPort, ib.Network, ib.Security, ib.WSPath, ib.HostHeader,
		ib.GRPCServiceName, ib.SNI, ib.Fingerprint, ib.RealityPublicKey,
		ib.RealityShortID, ib.Flow, ib.Method, ib.XHTTPMode, ib.Remark)
	return mapErr(err)
}

func (s *Store) GetInbound(ctx context.Context, id string) (*domain.Inbound, error) {
	return scanInbound(s.db.QueryRowContext(ctx,
		`SELECT `+inboundCols+` FROM inbounds WHERE id = $1`, id))
}

func (s *Store) ListInboundsByNode(ctx context.Context, nodeID string) ([]*domain.Inbound, error) {
	return s.queryInbounds(ctx,
		`SELECT `+inboundCols+` FROM inbounds WHERE node_id = $1 ORDER BY created_at`, nodeID)
}

// ListInboundsByIDs returns inbounds for the given ids (any order).
func (s *Store) ListInboundsByIDs(ctx context.Context, ids []string) ([]*domain.Inbound, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	return s.queryInbounds(ctx,
		`SELECT `+inboundCols+` FROM inbounds WHERE id = ANY($1)`, ids)
}

// ListInboundsForUser returns the inbounds a user is assigned to.
func (s *Store) ListInboundsForUser(ctx context.Context, userID string) ([]*domain.Inbound, error) {
	return s.queryInbounds(ctx, `
		SELECT `+inboundColsPrefixed("i")+` FROM inbounds i
		JOIN user_inbounds ui ON ui.inbound_id = i.id
		WHERE ui.user_id = $1 ORDER BY i.created_at`, userID)
}

func (s *Store) queryInbounds(ctx context.Context, q string, args ...any) ([]*domain.Inbound, error) {
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	var out []*domain.Inbound
	for rows.Next() {
		ib, err := scanInbound(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, ib)
	}
	return out, rows.Err()
}

func (s *Store) UpdateInbound(ctx context.Context, ib *domain.Inbound) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE inbounds SET tag=$2, protocol=$3, listen_port=$4, public_host=$5,
			public_port=$6, network=$7, security=$8, ws_path=$9, host_header=$10,
			grpc_service_name=$11, sni=$12, fingerprint=$13, reality_public_key=$14,
			reality_short_id=$15, flow=$16, method=$17, xhttp_mode=$18, remark=$19,
			updated_at=now()
		WHERE id = $1`,
		ib.ID, ib.Tag, ib.Protocol, ib.ListenPort, ib.PublicHost, ib.PublicPort,
		ib.Network, ib.Security, ib.WSPath, ib.HostHeader, ib.GRPCServiceName,
		ib.SNI, ib.Fingerprint, ib.RealityPublicKey, ib.RealityShortID, ib.Flow,
		ib.Method, ib.XHTTPMode, ib.Remark)
	return requireRow(res, err)
}

func (s *Store) DeleteInbound(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM inbounds WHERE id = $1`, id)
	return requireRow(res, err)
}

func inboundColsPrefixed(p string) string {
	return p + ".id, " + p + ".node_id, " + p + ".tag, " + p + ".protocol, " +
		p + ".listen_port, " + p + ".public_host, " + p + ".public_port, " +
		p + ".network, " + p + ".security, " + p + ".ws_path, " + p + ".host_header, " +
		p + ".grpc_service_name, " + p + ".sni, " + p + ".fingerprint, " +
		p + ".reality_public_key, " + p + ".reality_short_id, " + p + ".flow, " +
		p + ".method, " + p + ".xhttp_mode, " + p + ".remark, " +
		p + ".created_at, " + p + ".updated_at"
}
