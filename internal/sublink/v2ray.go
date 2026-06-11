package sublink

import (
	"encoding/base64"
	"strings"

	"github.com/ImErdis/xray-api/internal/domain"
)

// V2Ray renders the standard v2ray subscription: newline-joined share links,
// base64-encoded as a whole. One link per assigned inbound.
func V2Ray(u *domain.User, inbounds []*domain.Inbound) (string, error) {
	links := make([]string, 0, len(inbounds))
	for _, ib := range inbounds {
		l, err := Link(u, ib)
		if err != nil {
			return "", err
		}
		links = append(links, l)
	}
	joined := strings.Join(links, "\n")
	return base64.StdEncoding.EncodeToString([]byte(joined)), nil
}
