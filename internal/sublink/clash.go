package sublink

import (
	"github.com/ImErdis/xray-api/internal/domain"
	"gopkg.in/yaml.v3"
)

// clashProxy is a single proxy entry in a Clash.Meta config. Fields are a
// pragmatic subset covering vless/vmess/trojan over tcp/ws/grpc with tls/reality.
type clashProxy struct {
	Name        string            `yaml:"name"`
	Type        string            `yaml:"type"`
	Server      string            `yaml:"server"`
	Port        int               `yaml:"port"`
	UUID        string            `yaml:"uuid,omitempty"`
	Password    string            `yaml:"password,omitempty"`
	AlterID     *int              `yaml:"alterId,omitempty"`
	Cipher      string            `yaml:"cipher,omitempty"`
	Flow        string            `yaml:"flow,omitempty"`
	UDP         bool              `yaml:"udp"`
	TLS         bool              `yaml:"tls,omitempty"`
	Servername  string            `yaml:"servername,omitempty"`
	SNI         string            `yaml:"sni,omitempty"`
	Network     string            `yaml:"network,omitempty"`
	ClientFP    string            `yaml:"client-fingerprint,omitempty"`
	RealityOpts map[string]string `yaml:"reality-opts,omitempty"`
	WSOpts      map[string]any    `yaml:"ws-opts,omitempty"`
	GRPCOpts    map[string]string `yaml:"grpc-opts,omitempty"`
	XHTTPOpts   map[string]any    `yaml:"xhttp-opts,omitempty"`
}

type clashConfig struct {
	Proxies     []clashProxy     `yaml:"proxies"`
	ProxyGroups []map[string]any `yaml:"proxy-groups"`
	Rules       []string         `yaml:"rules"`
}

// Clash renders a minimal Clash.Meta config with one proxy per inbound and a
// select group, suitable for Clash Verge / Clash.Meta / Mihomo.
func Clash(u *domain.User, inbounds []*domain.Inbound) ([]byte, error) {
	cfg := clashConfig{
		Rules: []string{"MATCH,PROXY"},
	}
	var names []string
	for _, ib := range inbounds {
		p := clashProxyFor(u, ib)
		cfg.Proxies = append(cfg.Proxies, p)
		names = append(names, p.Name)
	}
	if len(names) == 0 {
		names = []string{"DIRECT"}
	}
	cfg.ProxyGroups = []map[string]any{
		{"name": "PROXY", "type": "select", "proxies": names},
	}
	return yaml.Marshal(cfg)
}

func clashProxyFor(u *domain.User, ib *domain.Inbound) clashProxy {
	p := clashProxy{
		Name:    remark(u, ib),
		Server:  ib.PublicHost,
		Port:    ib.PublicPort,
		UDP:     true,
		Network: string(ib.Network),
	}
	switch ib.Protocol {
	case domain.ProtocolVLESS:
		p.Type = "vless"
		p.UUID = u.UUID
		p.Flow = ib.Flow
	case domain.ProtocolVMess:
		p.Type = "vmess"
		p.UUID = u.UUID
		aid := 0
		p.AlterID = &aid
		p.Cipher = "auto"
	case domain.ProtocolTrojan:
		p.Type = "trojan"
		p.Password = u.TrojanPassword
	case domain.ProtocolShadowsocks:
		p.Type = "ss"
		p.Password = u.TrojanPassword
		p.Cipher = ib.Method
		if p.Cipher == "" {
			p.Cipher = "aes-256-gcm"
		}
		// Shadowsocks in Clash carries no TLS/transport block; emit the bare
		// proxy and return early.
		p.Network = ""
		return p
	}

	switch ib.Security {
	case domain.SecurityTLS:
		p.TLS = true
		p.SNI = ib.SNI
		p.Servername = ib.SNI
		p.ClientFP = ib.Fingerprint
	case domain.SecurityReality:
		p.TLS = true
		p.Servername = ib.SNI
		p.ClientFP = ib.Fingerprint
		p.RealityOpts = map[string]string{}
		if ib.RealityPublicKey != "" {
			p.RealityOpts["public-key"] = ib.RealityPublicKey
		}
		if ib.RealityShortID != "" {
			p.RealityOpts["short-id"] = ib.RealityShortID
		}
	}

	switch ib.Network {
	case domain.NetworkWS, domain.NetworkHTTPUpgrade:
		ws := map[string]any{}
		if ib.WSPath != "" {
			ws["path"] = ib.WSPath
		}
		if ib.HostHeader != "" {
			ws["headers"] = map[string]string{"Host": ib.HostHeader}
		}
		if len(ws) > 0 {
			p.WSOpts = ws
		}
	case domain.NetworkGRPC:
		if ib.GRPCServiceName != "" {
			p.GRPCOpts = map[string]string{"grpc-service-name": ib.GRPCServiceName}
		}
	case domain.NetworkXHTTP:
		xh := map[string]any{}
		if ib.WSPath != "" {
			xh["path"] = ib.WSPath
		}
		if ib.HostHeader != "" {
			xh["host"] = ib.HostHeader
		}
		if ib.XHTTPMode != "" {
			xh["mode"] = ib.XHTTPMode
		}
		if len(xh) > 0 {
			p.XHTTPOpts = xh
		}
	}
	return p
}
