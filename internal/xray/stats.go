package xray

import (
	"context"
	"fmt"
	"strings"

	scommand "github.com/xtls/xray-core/app/stats/command"
)

// QueryUserTraffic reads all "user>>>..." counters. With reset=true the node
// atomically zeroes them, so each read is an unambiguous delta for the interval
// since the previous read.
func (c *grpcClient) QueryUserTraffic(ctx context.Context, reset bool) ([]UserTraffic, error) {
	resp, err := c.stats.QueryStats(ctx, &scommand.QueryStatsRequest{
		Pattern: "user>>>",
		Reset_:  reset,
	})
	if err != nil {
		return nil, fmt.Errorf("query stats: %w", err)
	}
	byEmail := make(map[string]*UserTraffic)
	for _, st := range resp.GetStat() {
		email, dir, ok := parseUserStatName(st.GetName())
		if !ok {
			continue
		}
		ut := byEmail[email]
		if ut == nil {
			ut = &UserTraffic{Email: email}
			byEmail[email] = ut
		}
		switch dir {
		case "uplink":
			ut.Uplink += st.GetValue()
		case "downlink":
			ut.Downlink += st.GetValue()
		}
	}
	out := make([]UserTraffic, 0, len(byEmail))
	for _, ut := range byEmail {
		out = append(out, *ut)
	}
	return out, nil
}

// parseUserStatName splits "user>>>{email}>>>traffic>>>uplink" into its email
// and direction. The email itself may contain '>' only in pathological cases;
// we split on the fixed ">>>traffic>>>" separator to be safe.
func parseUserStatName(name string) (email, dir string, ok bool) {
	const prefix = "user>>>"
	const mid = ">>>traffic>>>"
	if !strings.HasPrefix(name, prefix) {
		return "", "", false
	}
	rest := name[len(prefix):]
	i := strings.LastIndex(rest, mid)
	if i < 0 {
		return "", "", false
	}
	email = rest[:i]
	dir = rest[i+len(mid):]
	if email == "" || (dir != "uplink" && dir != "downlink") {
		return "", "", false
	}
	return email, dir, true
}
