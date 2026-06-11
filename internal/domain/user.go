package domain

import "time"

// UserStatus is the lifecycle state of a subscription user.
type UserStatus string

const (
	UserStatusActive    UserStatus = "active"
	UserStatusSuspended UserStatus = "suspended" // over data quota
	UserStatusExpired   UserStatus = "expired"
	UserStatusDisabled  UserStatus = "disabled" // manually disabled by admin
)

// User is a provisioned subscriber. Email doubles as Xray's stats identity
// (counters are named user>>>{email}>>>traffic>>>...) and must be globally
// unique.
type User struct {
	ID             string     `json:"id"`
	Email          string     `json:"email"`
	UUID           string     `json:"uuid"`
	TrojanPassword string     `json:"trojan_password"`
	PlanID         *string    `json:"plan_id"`
	Status         UserStatus `json:"status"`
	DataLimitBytes *int64     `json:"data_limit_bytes"`

	UsedUploadBytes   int64 `json:"used_upload_bytes"`
	UsedDownloadBytes int64 `json:"used_download_bytes"`

	ExpiresAt *time.Time `json:"expires_at"`
	SubToken  string     `json:"-"`
	Note      string     `json:"note,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// UsedTotalBytes is the combined up+down usage counted against the quota.
func (u *User) UsedTotalBytes() int64 { return u.UsedUploadBytes + u.UsedDownloadBytes }

// OverQuota reports whether usage has reached the data limit (if any).
func (u *User) OverQuota() bool {
	return u.DataLimitBytes != nil && u.UsedTotalBytes() >= *u.DataLimitBytes
}

// SyncStatus tracks whether a user assignment has been pushed to a node.
type SyncStatus string

const (
	SyncPending  SyncStatus = "pending"
	SyncApplied  SyncStatus = "applied"
	SyncFailed   SyncStatus = "failed"
	SyncRemoving SyncStatus = "removing"
)

// UserInbound is the desired presence of a user on a specific inbound.
type UserInbound struct {
	UserID       string     `json:"user_id"`
	InboundID    string     `json:"inbound_id"`
	SyncStatus   SyncStatus `json:"sync_status"`
	LastSyncedAt *time.Time `json:"last_synced_at,omitempty"`
	LastError    string     `json:"last_error,omitempty"`
}

// TrafficSnapshot is one polling interval's traffic delta for a user on a node.
type TrafficSnapshot struct {
	ID            int64     `json:"id"`
	UserID        string    `json:"user_id"`
	NodeID        string    `json:"node_id"`
	UplinkBytes   int64     `json:"uplink_bytes"`
	DownlinkBytes int64     `json:"downlink_bytes"`
	CollectedAt   time.Time `json:"collected_at"`
}
