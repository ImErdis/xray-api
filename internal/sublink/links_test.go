package sublink

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/ImErdis/xray-api/internal/domain"
)

func vlessUser() *domain.User {
	return &domain.User{
		Email:          "alice@example.com",
		UUID:           "11111111-1111-1111-1111-111111111111",
		TrojanPassword: "s3cr3t",
	}
}

func TestVLESSLinkWSTLS(t *testing.T) {
	ib := &domain.Inbound{
		Tag:         "vless-ws",
		Protocol:    domain.ProtocolVLESS,
		PublicHost:  "edge.example.com",
		PublicPort:  443,
		Network:     domain.NetworkWS,
		Security:    domain.SecurityTLS,
		WSPath:      "/ws",
		HostHeader:  "edge.example.com",
		SNI:         "edge.example.com",
		Fingerprint: "chrome",
		Remark:      "US",
	}
	got, err := Link(vlessUser(), ib)
	if err != nil {
		t.Fatal(err)
	}
	want := "vless://11111111-1111-1111-1111-111111111111@edge.example.com:443" +
		"?encryption=none&fp=chrome&host=edge.example.com&path=%2Fws&security=tls" +
		"&sni=edge.example.com&type=ws#alice@example.com@US"
	if got != want {
		t.Errorf("vless link mismatch:\n got: %s\nwant: %s", got, want)
	}
}

func TestVLESSLinkRealityVision(t *testing.T) {
	ib := &domain.Inbound{
		Tag:              "vless-reality",
		Protocol:         domain.ProtocolVLESS,
		PublicHost:       "1.2.3.4",
		PublicPort:       443,
		Network:          domain.NetworkTCP,
		Security:         domain.SecurityReality,
		Flow:             "xtls-rprx-vision",
		SNI:              "www.microsoft.com",
		Fingerprint:      "chrome",
		RealityPublicKey: "PBK123",
		RealityShortID:   "ab12",
	}
	got, err := Link(vlessUser(), ib)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"security=reality", "flow=xtls-rprx-vision", "pbk=PBK123",
		"sid=ab12", "sni=www.microsoft.com", "fp=chrome",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("reality link missing %q in %s", want, got)
		}
	}
}

func TestTrojanLink(t *testing.T) {
	ib := &domain.Inbound{
		Tag:        "trojan",
		Protocol:   domain.ProtocolTrojan,
		PublicHost: "t.example.com",
		PublicPort: 443,
		Network:    domain.NetworkTCP,
		Security:   domain.SecurityTLS,
		SNI:        "t.example.com",
	}
	got, err := Link(vlessUser(), ib)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "trojan://s3cr3t@t.example.com:443") {
		t.Errorf("unexpected trojan link: %s", got)
	}
}

func TestVMessLinkDecodes(t *testing.T) {
	ib := &domain.Inbound{
		Tag:        "vmess-ws",
		Protocol:   domain.ProtocolVMess,
		PublicHost: "v.example.com",
		PublicPort: 80,
		Network:    domain.NetworkWS,
		Security:   domain.SecurityNone,
		WSPath:     "/vm",
	}
	got, err := Link(vlessUser(), ib)
	if err != nil {
		t.Fatal(err)
	}
	b64 := strings.TrimPrefix(got, "vmess://")
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		t.Fatalf("vmess payload not base64: %v", err)
	}
	for _, want := range []string{`"add":"v.example.com"`, `"port":"80"`, `"net":"ws"`, `"path":"/vm"`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("vmess json missing %q in %s", want, raw)
		}
	}
}

func TestV2RayBundleIsBase64OfLinks(t *testing.T) {
	ib := &domain.Inbound{
		Tag: "t", Protocol: domain.ProtocolTrojan,
		PublicHost: "h", PublicPort: 443, Network: domain.NetworkTCP, Security: domain.SecurityTLS,
	}
	out, err := V2Ray(vlessUser(), []*domain.Inbound{ib, ib})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := base64.StdEncoding.DecodeString(out)
	if err != nil {
		t.Fatalf("bundle not base64: %v", err)
	}
	if lines := strings.Split(string(raw), "\n"); len(lines) != 2 {
		t.Errorf("want 2 links, got %d", len(lines))
	}
}

func TestClashYAMLContainsProxy(t *testing.T) {
	ib := &domain.Inbound{
		Tag: "vless-ws", Protocol: domain.ProtocolVLESS,
		PublicHost: "edge", PublicPort: 443, Network: domain.NetworkWS,
		Security: domain.SecurityTLS, WSPath: "/ws", SNI: "edge",
	}
	out, err := Clash(vlessUser(), []*domain.Inbound{ib})
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, want := range []string{"proxies:", "type: vless", "network: ws", "PROXY"} {
		if !strings.Contains(s, want) {
			t.Errorf("clash yaml missing %q:\n%s", want, s)
		}
	}
}
