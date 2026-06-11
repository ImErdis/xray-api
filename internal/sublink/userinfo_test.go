package sublink

import (
	"testing"
	"time"

	"github.com/ImErdis/xray-api/internal/domain"
)

func TestUserInfoHeader(t *testing.T) {
	limit := int64(1000)
	exp := time.Unix(1700000000, 0)
	tests := []struct {
		name string
		u    *domain.User
		want string
	}{
		{
			name: "limited with expiry",
			u: &domain.User{
				UsedUploadBytes: 100, UsedDownloadBytes: 200,
				DataLimitBytes: &limit, ExpiresAt: &exp,
			},
			want: "upload=100; download=200; total=1000; expire=1700000000",
		},
		{
			name: "unlimited never",
			u:    &domain.User{UsedUploadBytes: 5, UsedDownloadBytes: 6},
			want: "upload=5; download=6; total=0; expire=0",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := UserInfoHeader(tt.u); got != tt.want {
				t.Errorf("got %q want %q", got, tt.want)
			}
		})
	}
}
