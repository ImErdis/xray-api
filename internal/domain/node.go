package domain

import "time"

// NodeStatus is the observed health of a managed Xray node.
type NodeStatus string

const (
	NodeStatusUnknown  NodeStatus = "unknown"
	NodeStatusOnline   NodeStatus = "online"
	NodeStatusOffline  NodeStatus = "offline"
	NodeStatusDisabled NodeStatus = "disabled"
)

// Node is a remote Xray-core instance managed over its gRPC API.
type Node struct {
	ID               string     `json:"id"`
	Name             string     `json:"name"`
	APIAddress       string     `json:"api_address"`
	APIPort          int        `json:"api_port"`
	APITLS           bool       `json:"api_tls"`
	APITLSServerName string     `json:"api_tls_server_name,omitempty"`
	APITLSInsecure   bool       `json:"api_tls_insecure"`
	Status           NodeStatus `json:"status"`
	LastSeenAt       *time.Time `json:"last_seen_at,omitempty"`
	LastError        string     `json:"last_error,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}
