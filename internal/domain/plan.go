package domain

import "time"

// Plan is a sellable subscription template. Nil limits mean unlimited /
// no expiry. Limits are snapshotted onto users at creation time.
type Plan struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	DataLimitBytes *int64    `json:"data_limit_bytes"`
	DurationDays   *int      `json:"duration_days"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}
