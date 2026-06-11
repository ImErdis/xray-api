package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/ImErdis/xray-api/internal/domain"
)

// DesiredAssignment is a user→inbound→node tuple the reconciler needs to push.
// It joins the data required to build an Xray account in one row.
type DesiredAssignment struct {
	UserID     string
	InboundID  string
	NodeID     string
	InboundTag string
	Protocol   domain.Protocol
	Flow       string
	Email      string
	UUID       string
	TrojanPass string
	SyncStatus domain.SyncStatus
}

// AddAssignmentTx inserts a pending user_inbound row in a transaction.
func (s *Store) AddAssignmentTx(ctx context.Context, tx *sql.Tx, userID, inboundID string) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO user_inbounds (user_id, inbound_id, sync_status)
		VALUES ($1, $2, 'pending')
		ON CONFLICT (user_id, inbound_id) DO NOTHING`, userID, inboundID)
	return mapErr(err)
}

// ListAssignmentsByUser returns a user's assignments.
func (s *Store) ListAssignmentsByUser(ctx context.Context, userID string) ([]*domain.UserInbound, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT user_id, inbound_id, sync_status, last_synced_at, last_error
		FROM user_inbounds WHERE user_id = $1`, userID)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	var out []*domain.UserInbound
	for rows.Next() {
		var ui domain.UserInbound
		var synced sql.NullTime
		if err := rows.Scan(&ui.UserID, &ui.InboundID, &ui.SyncStatus,
			&synced, &ui.LastError); err != nil {
			return nil, mapErr(err)
		}
		if synced.Valid {
			ui.LastSyncedAt = &synced.Time
		}
		out = append(out, &ui)
	}
	return out, rows.Err()
}

// MarkAssignmentSynced sets sync_status and clears/sets last_error.
func (s *Store) MarkAssignmentSynced(ctx context.Context, userID, inboundID string, status domain.SyncStatus, errMsg string) error {
	var seenAt *time.Time
	if status == domain.SyncApplied {
		now := time.Now()
		seenAt = &now
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE user_inbounds SET sync_status=$3, last_error=$4,
			last_synced_at = COALESCE($5, last_synced_at)
		WHERE user_id=$1 AND inbound_id=$2`,
		userID, inboundID, status, errMsg, seenAt)
	return mapErr(err)
}

// MarkAssignmentsRemovingForUser tombstones all of a user's assignments.
func (s *Store) MarkAssignmentsRemovingForUser(ctx context.Context, userID string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE user_inbounds SET sync_status='removing' WHERE user_id=$1`, userID)
	return mapErr(err)
}

// DeleteAssignment removes a single user_inbound row (after a successful remove).
func (s *Store) DeleteAssignment(ctx context.Context, userID, inboundID string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM user_inbounds WHERE user_id=$1 AND inbound_id=$2`, userID, inboundID)
	return mapErr(err)
}

// SetAssignmentsForUserTx replaces a user's assignment set in a transaction:
// inserts missing ones as pending and tombstones removed ones as 'removing'.
func (s *Store) SetAssignmentsForUserTx(ctx context.Context, tx *sql.Tx, userID string, inboundIDs []string) error {
	if _, err := tx.ExecContext(ctx, `
		UPDATE user_inbounds SET sync_status='removing'
		WHERE user_id=$1 AND inbound_id <> ALL($2)`, userID, inboundIDs); err != nil {
		return mapErr(err)
	}
	for _, ibID := range inboundIDs {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO user_inbounds (user_id, inbound_id, sync_status)
			VALUES ($1, $2, 'pending')
			ON CONFLICT (user_id, inbound_id) DO NOTHING`, userID, ibID); err != nil {
			return mapErr(err)
		}
	}
	return nil
}

// DesiredForNode returns assignments that should be present on a node:
// active users with non-removing assignments. Used by the reconciler to push.
func (s *Store) DesiredForNode(ctx context.Context, nodeID string) ([]*DesiredAssignment, error) {
	return s.queryDesired(ctx, `
		SELECT ui.user_id, ui.inbound_id, i.node_id, i.tag, i.protocol, i.flow,
			u.email, u.uuid, u.trojan_password, ui.sync_status
		FROM user_inbounds ui
		JOIN inbounds i ON i.id = ui.inbound_id
		JOIN users u ON u.id = ui.user_id
		WHERE i.node_id = $1 AND u.status = 'active' AND ui.sync_status <> 'removing'`,
		nodeID)
}

// RemovalsForNode returns assignments that should be absent on a node:
// either tombstoned, or belonging to a non-active user.
func (s *Store) RemovalsForNode(ctx context.Context, nodeID string) ([]*DesiredAssignment, error) {
	return s.queryDesired(ctx, `
		SELECT ui.user_id, ui.inbound_id, i.node_id, i.tag, i.protocol, i.flow,
			u.email, u.uuid, u.trojan_password, ui.sync_status
		FROM user_inbounds ui
		JOIN inbounds i ON i.id = ui.inbound_id
		JOIN users u ON u.id = ui.user_id
		WHERE i.node_id = $1 AND (ui.sync_status = 'removing' OR u.status <> 'active')`,
		nodeID)
}

func (s *Store) queryDesired(ctx context.Context, q string, args ...any) ([]*DesiredAssignment, error) {
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	var out []*DesiredAssignment
	for rows.Next() {
		var d DesiredAssignment
		if err := rows.Scan(&d.UserID, &d.InboundID, &d.NodeID, &d.InboundTag,
			&d.Protocol, &d.Flow, &d.Email, &d.UUID, &d.TrojanPass, &d.SyncStatus); err != nil {
			return nil, mapErr(err)
		}
		out = append(out, &d)
	}
	return out, rows.Err()
}

// AssignmentTargetsForUser returns every (node, inbound, account) tuple a user
// is assigned to, regardless of status — used to remove a user everywhere on
// suspend/expire/delete.
func (s *Store) AssignmentTargetsForUser(ctx context.Context, userID string) ([]*DesiredAssignment, error) {
	return s.queryDesired(ctx, `
		SELECT ui.user_id, ui.inbound_id, i.node_id, i.tag, i.protocol, i.flow,
			u.email, u.uuid, u.trojan_password, ui.sync_status
		FROM user_inbounds ui
		JOIN inbounds i ON i.id = ui.inbound_id
		JOIN users u ON u.id = ui.user_id
		WHERE ui.user_id = $1`, userID)
}
