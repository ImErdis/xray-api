package xray

import "testing"

func TestParseUserStatName(t *testing.T) {
	tests := []struct {
		name      string
		wantEmail string
		wantDir   string
		wantOK    bool
	}{
		{"user>>>alice@example.com>>>traffic>>>uplink", "alice@example.com", "uplink", true},
		{"user>>>bob@x.io>>>traffic>>>downlink", "bob@x.io", "downlink", true},
		{"inbound>>>api>>>traffic>>>uplink", "", "", false},
		{"user>>>x>>>traffic>>>sideways", "", "", false},
		{"user>>>>>>traffic>>>uplink", "", "", false},
		{"garbage", "", "", false},
	}
	for _, tt := range tests {
		email, dir, ok := parseUserStatName(tt.name)
		if ok != tt.wantOK || email != tt.wantEmail || dir != tt.wantDir {
			t.Errorf("%q => (%q,%q,%v), want (%q,%q,%v)",
				tt.name, email, dir, ok, tt.wantEmail, tt.wantDir, tt.wantOK)
		}
	}
}
