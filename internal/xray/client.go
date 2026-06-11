// Package xray is the sole boundary to github.com/xtls/xray-core. Everything
// outside this package depends only on the Client interface, so the heavy
// upstream dependency can be swapped for generated protobuf stubs later
// without touching the rest of the codebase.
package xray

import (
	"context"
	"crypto/tls"
	"fmt"
	"time"

	hcommand "github.com/xtls/xray-core/app/proxyman/command"
	scommand "github.com/xtls/xray-core/app/stats/command"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

// Account is the protocol-specific identity to provision on an inbound.
type Account struct {
	Protocol string // vless|vmess|trojan
	Email    string
	UUID     string // vless/vmess
	Password string // trojan
	Flow     string // vless only
}

// UserTraffic is one user's up/down counters from a stats query.
type UserTraffic struct {
	Email    string
	Uplink   int64
	Downlink int64
}

// Client talks to a single Xray node's gRPC API.
type Client interface {
	// Ping verifies connectivity (and implicitly that StatsService is up).
	Ping(ctx context.Context) error
	// AddUser provisions an account on the given inbound tag. Idempotent:
	// an already-present user is treated as success.
	AddUser(ctx context.Context, inboundTag string, acc Account) error
	// RemoveUser removes a user (by email) from the inbound tag. Idempotent:
	// an absent user is treated as success.
	RemoveUser(ctx context.Context, inboundTag, email string) error
	// QueryUserTraffic returns all per-user counters, optionally resetting
	// them to zero (reset-on-read delta accounting).
	QueryUserTraffic(ctx context.Context, reset bool) ([]UserTraffic, error)
	Close() error
}

// DialOptions configures a node gRPC connection.
type DialOptions struct {
	Address       string
	Port          int
	TLS           bool
	TLSServerName string
	TLSInsecure   bool
	DialTimeout   time.Duration
}

type grpcClient struct {
	conn    *grpc.ClientConn
	handler hcommand.HandlerServiceClient
	stats   scommand.StatsServiceClient
}

// Dial opens a connection to a node. The connection is lazy; Ping forces it.
func Dial(opts DialOptions) (Client, error) {
	var creds credentials.TransportCredentials
	if opts.TLS {
		tc := &tls.Config{ServerName: opts.TLSServerName, InsecureSkipVerify: opts.TLSInsecure}
		creds = credentials.NewTLS(tc)
	} else {
		creds = insecure.NewCredentials()
	}
	target := fmt.Sprintf("%s:%d", opts.Address, opts.Port)
	conn, err := grpc.NewClient(target, grpc.WithTransportCredentials(creds))
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", target, err)
	}
	return &grpcClient{
		conn:    conn,
		handler: hcommand.NewHandlerServiceClient(conn),
		stats:   scommand.NewStatsServiceClient(conn),
	}, nil
}

func (c *grpcClient) Close() error { return c.conn.Close() }

func (c *grpcClient) Ping(ctx context.Context) error {
	_, err := c.stats.GetSysStats(ctx, &scommand.SysStatsRequest{})
	if err != nil {
		return fmt.Errorf("get sys stats: %w", err)
	}
	return nil
}
