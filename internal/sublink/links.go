// Package sublink builds client-importable proxy links and subscription
// payloads (v2ray base64 bundle, Clash YAML) from users and inbounds.
package sublink

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strconv"

	"github.com/ImErdis/xray-api/internal/domain"
)

// Link builds a single share URI for a user on an inbound. The remark (URI
// fragment) defaults to "{email}@{inbound.Remark or tag}".
func Link(u *domain.User, ib *domain.Inbound) (string, error) {
	switch ib.Protocol {
	case domain.ProtocolVLESS:
		return vlessLink(u, ib), nil
	case domain.ProtocolTrojan:
		return trojanLink(u, ib), nil
	case domain.ProtocolVMess:
		return vmessLink(u, ib), nil
	case domain.ProtocolShadowsocks:
		return shadowsocksLink(u, ib), nil
	default:
		return "", fmt.Errorf("unsupported protocol %q", ib.Protocol)
	}
}

func remark(u *domain.User, ib *domain.Inbound) string {
	label := ib.Remark
	if label == "" {
		label = ib.Tag
	}
	return u.Email + "@" + label
}

func hostPort(ib *domain.Inbound) string {
	return net.JoinHostPort(ib.PublicHost, strconv.Itoa(ib.PublicPort))
}

// streamParams are the transport/security query params shared by vless/trojan.
func streamParams(ib *domain.Inbound) url.Values {
	q := url.Values{}
	q.Set("type", string(ib.Network))
	q.Set("security", string(ib.Security))

	switch ib.Network {
	case domain.NetworkWS, domain.NetworkHTTPUpgrade:
		if ib.WSPath != "" {
			q.Set("path", ib.WSPath)
		}
		if ib.HostHeader != "" {
			q.Set("host", ib.HostHeader)
		}
	case domain.NetworkGRPC:
		if ib.GRPCServiceName != "" {
			q.Set("serviceName", ib.GRPCServiceName)
		}
	case domain.NetworkXHTTP:
		// XHTTP reuses the path/host fields; mode defaults to auto client-side.
		if ib.WSPath != "" {
			q.Set("path", ib.WSPath)
		}
		if ib.HostHeader != "" {
			q.Set("host", ib.HostHeader)
		}
		if ib.XHTTPMode != "" {
			q.Set("mode", ib.XHTTPMode)
		}
	}

	switch ib.Security {
	case domain.SecurityTLS:
		if ib.SNI != "" {
			q.Set("sni", ib.SNI)
		}
		if ib.Fingerprint != "" {
			q.Set("fp", ib.Fingerprint)
		}
	case domain.SecurityReality:
		if ib.SNI != "" {
			q.Set("sni", ib.SNI)
		}
		if ib.Fingerprint != "" {
			q.Set("fp", ib.Fingerprint)
		}
		if ib.RealityPublicKey != "" {
			q.Set("pbk", ib.RealityPublicKey)
		}
		if ib.RealityShortID != "" {
			q.Set("sid", ib.RealityShortID)
		}
	}
	return q
}

func vlessLink(u *domain.User, ib *domain.Inbound) string {
	q := streamParams(ib)
	q.Set("encryption", "none")
	if ib.Flow != "" {
		q.Set("flow", ib.Flow)
	}
	uri := url.URL{
		Scheme:   "vless",
		User:     url.User(u.UUID),
		Host:     hostPort(ib),
		RawQuery: encodeSorted(q),
		Fragment: remark(u, ib),
	}
	return uri.String()
}

func trojanLink(u *domain.User, ib *domain.Inbound) string {
	q := streamParams(ib)
	uri := url.URL{
		Scheme:   "trojan",
		User:     url.User(u.TrojanPassword),
		Host:     hostPort(ib),
		RawQuery: encodeSorted(q),
		Fragment: remark(u, ib),
	}
	return uri.String()
}

// shadowsocksLink encodes the SIP002 ss:// format:
//
//	ss://base64url(method:password)@host:port#remark
//
// Transport is plain TCP (the common SS deployment); Xray SS over ws/grpc is
// uncommon and omitted from the link for client compatibility.
func shadowsocksLink(u *domain.User, ib *domain.Inbound) string {
	method := ib.Method
	if method == "" {
		method = "aes-256-gcm"
	}
	userinfo := base64.RawURLEncoding.EncodeToString([]byte(method + ":" + u.TrojanPassword))
	uri := url.URL{
		Scheme:   "ss",
		Host:     hostPort(ib),
		Fragment: remark(u, ib),
	}
	// SIP002 puts the base64 method:password in the userinfo, unencoded by
	// url.URL (which would percent-encode it), so assemble manually.
	return "ss://" + userinfo + "@" + uri.Host + "#" + url.PathEscape(remark(u, ib))
}

// vmessLink encodes the classic base64-JSON vmess:// format (v2rayN schema v2).
func vmessLink(u *domain.User, ib *domain.Inbound) string {
	m := map[string]string{
		"v":    "2",
		"ps":   remark(u, ib),
		"add":  ib.PublicHost,
		"port": strconv.Itoa(ib.PublicPort),
		"id":   u.UUID,
		"aid":  "0",
		"scy":  "auto",
		"net":  string(ib.Network),
		"type": "none",
		"tls":  tlsField(ib.Security),
	}
	switch ib.Network {
	case domain.NetworkWS, domain.NetworkHTTPUpgrade:
		m["path"] = ib.WSPath
		m["host"] = ib.HostHeader
	case domain.NetworkGRPC:
		m["path"] = ib.GRPCServiceName
	}
	if ib.Security != domain.SecurityNone && ib.SNI != "" {
		m["sni"] = ib.SNI
	}
	b, _ := json.Marshal(m)
	return "vmess://" + base64.StdEncoding.EncodeToString(b)
}

func tlsField(s domain.Security) string {
	if s == domain.SecurityNone {
		return ""
	}
	return string(s)
}

// encodeSorted renders query params with deterministic key order so that
// generated links are stable (important for golden tests and caching).
func encodeSorted(v url.Values) string {
	return v.Encode() // url.Values.Encode already sorts keys
}
