//go:build integration

// Package integration exercises the control plane against a real Xray-core
// node. Run with: make integration
//
// It expects an Xray node reachable over gRPC, configured per
// docs/example-node-config.json. Point it at one with:
//
//	XRAY_TEST_ADDR=127.0.0.1 XRAY_TEST_PORT=10085 \
//	XRAY_TEST_INBOUND_TAG=vless-ws go test -tags integration ./test/integration/...
//
// The docker-compose.dev.yml stack provides exactly this (xray:10085).
package integration

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ImErdis/xray-api/internal/xray"
)

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func dialTestNode(t *testing.T) xray.Client {
	t.Helper()
	addr := os.Getenv("XRAY_TEST_ADDR")
	if addr == "" {
		t.Skip("XRAY_TEST_ADDR not set; skipping live Xray integration test")
	}
	port, _ := strconv.Atoi(os.Getenv("XRAY_TEST_PORT"))
	if port == 0 {
		port = 10085
	}
	c, err := xray.Dial(xray.DialOptions{Address: addr, Port: port})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

func TestLiveAddRemoveAndStats(t *testing.T) {
	c := dialTestNode(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	uptime, err := c.Ping(ctx)
	if err != nil {
		t.Fatalf("ping: %v", err)
	}
	if uptime == 0 {
		t.Error("expected non-zero uptime from a running node")
	}

	tag := os.Getenv("XRAY_TEST_INBOUND_TAG")
	if tag == "" {
		tag = "vless-ws"
	}
	// Protocol/method follow the inbound the tag points at, so the same test
	// exercises whichever inbound the operator configured (vless/trojan/
	// shadowsocks, any transport).
	acc := xray.Account{
		Protocol: envOr("XRAY_TEST_PROTOCOL", "vless"),
		Email:    "it-" + uuid.NewString() + "@test",
		UUID:     uuid.NewString(),
		Password: uuid.NewString(),
		Method:   envOr("XRAY_TEST_METHOD", "aes-256-gcm"),
	}

	if err := c.AddUser(ctx, tag, acc); err != nil {
		t.Fatalf("add user: %v", err)
	}
	// Idempotent re-add must succeed.
	if err := c.AddUser(ctx, tag, acc); err != nil {
		t.Fatalf("re-add user (should be idempotent): %v", err)
	}

	// Stats query should not error (counters may be zero with no traffic).
	if _, err := c.QueryUserTraffic(ctx, true); err != nil {
		t.Fatalf("query traffic: %v", err)
	}

	if err := c.RemoveUser(ctx, tag, acc.Email); err != nil {
		t.Fatalf("remove user: %v", err)
	}
	// Idempotent re-remove must succeed.
	if err := c.RemoveUser(ctx, tag, acc.Email); err != nil {
		t.Fatalf("re-remove user (should be idempotent): %v", err)
	}
}
