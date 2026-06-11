package xray

import (
	"testing"

	"github.com/xtls/xray-core/proxy/shadowsocks"
)

func TestShadowsocksCipher(t *testing.T) {
	tests := []struct {
		method  string
		want    shadowsocks.CipherType
		wantErr bool
	}{
		{"aes-128-gcm", shadowsocks.CipherType_AES_128_GCM, false},
		{"aes-256-gcm", shadowsocks.CipherType_AES_256_GCM, false},
		{"chacha20-ietf-poly1305", shadowsocks.CipherType_CHACHA20_POLY1305, false},
		{"chacha20-poly1305", shadowsocks.CipherType_CHACHA20_POLY1305, false},
		{"xchacha20-ietf-poly1305", shadowsocks.CipherType_XCHACHA20_POLY1305, false},
		{"none", shadowsocks.CipherType_NONE, false},
		{"rc4-md5", shadowsocks.CipherType_UNKNOWN, true},
	}
	for _, tt := range tests {
		got, err := shadowsocksCipher(tt.method)
		if (err != nil) != tt.wantErr || got != tt.want {
			t.Errorf("shadowsocksCipher(%q) = (%v,%v), want (%v,err=%v)",
				tt.method, got, err, tt.want, tt.wantErr)
		}
	}
}

func TestBuildUserShadowsocks(t *testing.T) {
	u, err := buildUser(Account{
		Protocol: "shadowsocks", Email: "x@y", Password: "pw", Method: "aes-256-gcm",
	})
	if err != nil {
		t.Fatal(err)
	}
	if u.Email != "x@y" || u.Account == nil {
		t.Errorf("unexpected user: %+v", u)
	}
}
