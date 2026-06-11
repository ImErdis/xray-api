package store

import (
	"context"
	"database/sql"
	"time"
)

// TrafficDelta is one user's traffic increment for a collection cycle.
type TrafficDelta struct {
	UserID   string
	Uplink   int64
	Downlink int64
}

// ApplyTrafficDeltas, in a single transaction, records snapshot rows and
// increments users' running totals, then returns the ids of active users that
// have crossed their data limit (so the caller can suspend + remove them).
func (s *Store) ApplyTrafficDeltas(ctx context.Context, nodeID string, deltas []TrafficDelta, at time.Time) ([]string, error) {
	var overQuota []string
	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		for _, d := range deltas {
			if d.Uplink == 0 && d.Downlink == 0 {
				continue
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO traffic_snapshots (user_id, node_id, uplink_bytes, downlink_bytes, collected_at)
				VALUES ($1, $2, $3, $4, $5)`,
				d.UserID, nodeID, d.Uplink, d.Downlink, at); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `
				UPDATE users SET used_upload_bytes = used_upload_bytes + $2,
					used_download_bytes = used_download_bytes + $3, updated_at = now()
				WHERE id = $1`, d.UserID, d.Uplink, d.Downlink); err != nil {
				return err
			}
		}
		// Flip newly over-quota active users to suspended, atomically.
		rows, err := tx.QueryContext(ctx, `
			UPDATE users SET status = 'suspended', updated_at = now()
			WHERE status = 'active' AND data_limit_bytes IS NOT NULL
			  AND used_upload_bytes + used_download_bytes >= data_limit_bytes
			RETURNING id`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				return err
			}
			overQuota = append(overQuota, id)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return overQuota, nil
}

// UsageBucket is an aggregated usage row for the usage endpoint.
type UsageBucket struct {
	Key      string `json:"key"` // day (RFC3339 date) or node id
	Uplink   int64  `json:"uplink_bytes"`
	Downlink int64  `json:"downlink_bytes"`
}

// UserUsage aggregates a user's snapshots between from and to, grouped by day
// or node.
func (s *Store) UserUsage(ctx context.Context, userID string, from, to time.Time, groupBy string) ([]UsageBucket, error) {
	var keyExpr string
	switch groupBy {
	case "node":
		keyExpr = "node_id::text"
	default:
		keyExpr = "to_char(date_trunc('day', collected_at), 'YYYY-MM-DD')"
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+keyExpr+` AS k, COALESCE(sum(uplink_bytes),0), COALESCE(sum(downlink_bytes),0)
		FROM traffic_snapshots
		WHERE user_id = $1 AND collected_at >= $2 AND collected_at < $3
		GROUP BY k ORDER BY k`, userID, from, to)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	var out []UsageBucket
	for rows.Next() {
		var b UsageBucket
		if err := rows.Scan(&b.Key, &b.Uplink, &b.Downlink); err != nil {
			return nil, mapErr(err)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// PruneSnapshots deletes snapshots older than the cutoff; returns rows removed.
func (s *Store) PruneSnapshots(ctx context.Context, before time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM traffic_snapshots WHERE collected_at < $1`, before)
	if err != nil {
		return 0, mapErr(err)
	}
	return res.RowsAffected()
}
