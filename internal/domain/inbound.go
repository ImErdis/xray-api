package domain

import "time"

// Protocol is an Xray proxy protocol supported for user provisioning.
type Protocol string

const (
	ProtocolVLESS  Protocol = "vless"
	ProtocolVMess  Protocol = "vmess"
	ProtocolTrojan Protocol = "trojan"
)

func (p Protocol) Valid() bool {
	switch p {
	case ProtocolVLESS, ProtocolVMess, ProtocolTrojan:
		return true
	}
	return false
}

// Network is the stream transport of an inbound.
type Network string

const (
	NetworkTCP         Network = "tcp"
	NetworkWS          Network = "ws"
	NetworkGRPC        Network = "grpc"
	NetworkHTTPUpgrade Network = "httpupgrade"
)

func (n Network) Valid() bool {
	switch n {
	case NetworkTCP, NetworkWS, NetworkGRPC, NetworkHTTPUpgrade:
		return true
	}
	return false
}

// Security is the stream security layer of an inbound.
type Security string

const (
	SecurityNone    Security = "none"
	SecurityTLS     Security = "tls"
	SecurityReality Security = "reality"
)

func (s Security) Valid() bool {
	switch s {
	case SecurityNone, SecurityTLS, SecurityReality:
		return true
	}
	return false
}

// Inbound mirrors an inbound configured in a node's Xray config file.
// Tag must match the tag in the node config; the control plane manages the
// inbound's client list at runtime but never creates the inbound itself.
type Inbound struct {
	ID         string   `json:"id"`
	NodeID     string   `json:"node_id"`
	Tag        string   `json:"tag"`
	Protocol   Protocol `json:"protocol"`
	ListenPort int      `json:"listen_port"`

	// Public connection info advertised in subscription links; may differ
	// from listen_port when the node sits behind a CDN or port forward.
	PublicHost string `json:"public_host"`
	PublicPort int    `json:"public_port"`

	Network  Network  `json:"network"`
	Security Security `json:"security"`

	WSPath           string `json:"ws_path,omitempty"`
	HostHeader       string `json:"host_header,omitempty"`
	GRPCServiceName  string `json:"grpc_service_name,omitempty"`
	SNI              string `json:"sni,omitempty"`
	Fingerprint      string `json:"fingerprint,omitempty"`
	RealityPublicKey string `json:"reality_public_key,omitempty"`
	RealityShortID   string `json:"reality_short_id,omitempty"`
	Flow             string `json:"flow,omitempty"`

	Remark    string    `json:"remark,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
