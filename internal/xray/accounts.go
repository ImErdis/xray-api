package xray

import (
	"fmt"

	"github.com/xtls/xray-core/common/protocol"
	"github.com/xtls/xray-core/common/serial"
	"github.com/xtls/xray-core/proxy/shadowsocks"
	"github.com/xtls/xray-core/proxy/trojan"
	"github.com/xtls/xray-core/proxy/vless"
	"github.com/xtls/xray-core/proxy/vmess"
)

// buildUser turns a domain account into the protocol.User that AlterInbound's
// AddUserOperation expects. Email is the stats identity and must be set.
func buildUser(acc Account) (*protocol.User, error) {
	u := &protocol.User{Email: acc.Email, Level: 0}
	switch acc.Protocol {
	case "vless":
		u.Account = serial.ToTypedMessage(&vless.Account{
			Id:   acc.UUID,
			Flow: acc.Flow,
		})
	case "vmess":
		u.Account = serial.ToTypedMessage(&vmess.Account{
			Id: acc.UUID,
		})
	case "trojan":
		u.Account = serial.ToTypedMessage(&trojan.Account{
			Password: acc.Password,
		})
	case "shadowsocks":
		cipher, err := shadowsocksCipher(acc.Method)
		if err != nil {
			return nil, err
		}
		u.Account = serial.ToTypedMessage(&shadowsocks.Account{
			Password:   acc.Password,
			CipherType: cipher,
		})
	default:
		return nil, fmt.Errorf("unsupported protocol %q", acc.Protocol)
	}
	return u, nil
}

// shadowsocksCipher maps a method string to Xray's CipherType. It accepts both
// the canonical SS names and Xray's aliases so an inbound configured either way
// still matches.
func shadowsocksCipher(method string) (shadowsocks.CipherType, error) {
	switch method {
	case "aes-128-gcm", "aead_aes_128_gcm":
		return shadowsocks.CipherType_AES_128_GCM, nil
	case "aes-256-gcm", "aead_aes_256_gcm":
		return shadowsocks.CipherType_AES_256_GCM, nil
	case "chacha20-ietf-poly1305", "chacha20-poly1305", "aead_chacha20_poly1305":
		return shadowsocks.CipherType_CHACHA20_POLY1305, nil
	case "xchacha20-ietf-poly1305", "xchacha20-poly1305", "aead_xchacha20_poly1305":
		return shadowsocks.CipherType_XCHACHA20_POLY1305, nil
	case "none", "plain":
		return shadowsocks.CipherType_NONE, nil
	default:
		return shadowsocks.CipherType_UNKNOWN, fmt.Errorf("unsupported shadowsocks method %q", method)
	}
}
