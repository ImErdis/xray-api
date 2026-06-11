// Package xray is the sole boundary to github.com/xtls/xray-core. Everything
// outside this package depends only on the Client interface, so the heavy
// upstream dependency can be swapped for generated protobuf stubs later
// without touching the rest of the codebase.
package xray

import (
	"context"
	"crypto/tls"
	"crypto/x509"
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
	Protocol string // vless|vmess|trojan|shadowsocks
	Email    string
	UUID     string // vless/vmess
	Password string // trojan/shadowsocks
	Flow     string // vless only
	Method   string // shadowsocks cipher
}

// UserTraffic is one user's up/down counters from a stats query.
type UserTraffic struct {
	Email    string
	Uplink   int64
	Downlink int64
}

// Client talks to a single Xray node's gRPC API.
type Client interface {
	// Ping verifies connectivity and returns the node's uptime in seconds.
	// A decreasing uptime between pings means Xray restarted (and wiped all
	// runtime-added users), so the caller must reconcile.
	Ping(ctx context.Context) (uptimeSeconds uint32, err error)
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

	// mTLS: when ClientCertPEM/ClientKeyPEM are set, the control plane presents
	// a client certificate the node can verify. When CACertPEM is set, the
	// node's server certificate is verified against it instead of the system
	// roots (certificate pinning). All are PEM-encoded.
	CACertPEM     string
	ClientCertPEM string
	ClientKeyPEM  string
}

type grpcClient struct {
	conn    *grpc.ClientConn
	handler hcommand.HandlerServiceClient
	stats   scommand.StatsServiceClient
}

// buildTLSConfig assembles the *tls.Config for a node connection: optional CA
// pinning and optional client-certificate (mTLS) presentation.
func buildTLSConfig(opts DialOptions) (*tls.Config, error) {
	tc := &tls.Config{
		ServerName:         opts.TLSServerName,
		InsecureSkipVerify: opts.TLSInsecure,
		MinVersion:         tls.VersionTLS12,
	}
	if opts.CACertPEM != "" {
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM([]byte(opts.CACertPEM)) {
			return nil, fmt.Errorf("ca_cert: no valid certificate found in PEM")
		}
		tc.RootCAs = pool
	}
	if opts.ClientCertPEM != "" || opts.ClientKeyPEM != "" {
		if opts.ClientCertPEM == "" || opts.ClientKeyPEM == "" {
			return nil, fmt.Errorf("client cert and key must be provided together")
		}
		cert, err := tls.X509KeyPair([]byte(opts.ClientCertPEM), []byte(opts.ClientKeyPEM))
		if err != nil {
			return nil, fmt.Errorf("client cert/key: %w", err)
		}
		tc.Certificates = []tls.Certificate{cert}
	}
	return tc, nil
}

// ValidateTLSMaterial checks that the supplied PEM CA/cert/key parse and are
// internally consistent, without opening a connection. Used to fail fast on
// misconfigured node credentials.
func ValidateTLSMaterial(caPEM, clientCertPEM, clientKeyPEM string) error {
	_, err := buildTLSConfig(DialOptions{
		TLS:           true,
		CACertPEM:     caPEM,
		ClientCertPEM: clientCertPEM,
		ClientKeyPEM:  clientKeyPEM,
	})
	return err
}

// Dial opens a connection to a node. The connection is lazy; Ping forces it.
func Dial(opts DialOptions) (Client, error) {
	var creds credentials.TransportCredentials
	if opts.TLS {
		tc, err := buildTLSConfig(opts)
		if err != nil {
			return nil, err
		}
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

func (c *grpcClient) Ping(ctx context.Context) (uint32, error) {
	resp, err := c.stats.GetSysStats(ctx, &scommand.SysStatsRequest{})
	if err != nil {
		return 0, fmt.Errorf("get sys stats: %w", err)
	}
	return resp.GetUptime(), nil
}
