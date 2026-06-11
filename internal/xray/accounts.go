package xray

import (
	"fmt"

	"github.com/xtls/xray-core/common/protocol"
	"github.com/xtls/xray-core/common/serial"
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
	default:
		return nil, fmt.Errorf("unsupported protocol %q", acc.Protocol)
	}
	return u, nil
}
