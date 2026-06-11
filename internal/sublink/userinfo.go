package sublink

import (
	"fmt"

	"github.com/ImErdis/xray-api/internal/domain"
)

// UserInfoHeader builds the value of the subscription-userinfo header consumed
// by clients (v2rayN, Clash Verge, etc.) to render usage/quota/expiry.
// total=0 means unlimited; expire=0 means never.
func UserInfoHeader(u *domain.User) string {
	var total int64
	if u.DataLimitBytes != nil {
		total = *u.DataLimitBytes
	}
	var expire int64
	if u.ExpiresAt != nil {
		expire = u.ExpiresAt.Unix()
	}
	return fmt.Sprintf("upload=%d; download=%d; total=%d; expire=%d",
		u.UsedUploadBytes, u.UsedDownloadBytes, total, expire)
}
