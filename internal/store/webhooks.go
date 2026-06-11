package store

import (
	"context"
)

// RecordWebhookEvent inserts the provider event id and reports whether it was
// new. A false return means the event was already processed (replay/retry) and
// the caller must skip it.
func (s *Store) RecordWebhookEvent(ctx context.Context, eventID, action string) (bool, error) {
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO webhook_events (id, action) VALUES ($1, $2)
		ON CONFLICT (id) DO NOTHING`, eventID, action)
	if err != nil {
		return false, mapErr(err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n == 1, nil
}
