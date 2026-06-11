package httpapi

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ImErdis/xray-api/internal/config"
	"github.com/ImErdis/xray-api/internal/domain"
	"github.com/ImErdis/xray-api/internal/service"
	"github.com/ImErdis/xray-api/internal/store"
	"github.com/ImErdis/xray-api/internal/worker"
	"github.com/ImErdis/xray-api/internal/xray"
)

const (
	testBootstrapKey = "test-bootstrap-key"
	testBillingKey   = "whsec_httptest"
)

// fakeDial keeps worker.Manager constructible without a reachable node.
func fakeDial(*domain.Node) (xray.Client, error) {
	return nil, context.DeadlineExceeded
}

type testEnv struct {
	srv       *httptest.Server
	store     *store.Store
	apiKeys   *service.APIKeyService
	inboundID string
}

// newTestEnv stands up the full router against the TEST_PG_DSN database with
// a fake node dialer, mirroring the production wiring in cmd/xray-api.
func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	dsn := os.Getenv("TEST_PG_DSN")
	if dsn == "" {
		t.Skip("TEST_PG_DSN not set; skipping Postgres integration test")
	}
	ctx := context.Background()
	st, err := store.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	mgr := worker.NewManager(st, fakeDial, worker.Config{
		HealthInterval: time.Minute, StatsInterval: time.Minute, ReconcileInterval: time.Hour,
	}, log)

	users := service.NewUserService(st, mgr, "http://test")
	cfg := config.Default()
	cfg.Auth.BootstrapAPIKey = testBootstrapKey
	cfg.Webhooks.BillingSecret = testBillingKey
	cfg.RateLimit.PublicPerMinute = 0
	cfg.Metrics.Enabled = false

	s := NewServer(Deps{
		Store:   st,
		Nodes:   service.NewNodeService(st, mgr, fakeDial),
		Plans:   service.NewPlanService(st),
		Users:   users,
		APIKeys: service.NewAPIKeyService(st),
		Billing: service.NewBillingService(st, users),
		Log:     log,
		Config:  cfg,
	})
	srv := httptest.NewServer(s.Router())
	t.Cleanup(srv.Close)

	// One node + inbound to provision users against.
	node := &domain.Node{ID: uuid.NewString(), Name: "ht-" + uuid.NewString(),
		APIAddress: "x", APIPort: 1, Status: domain.NodeStatusUnknown}
	if err := st.CreateNode(ctx, node); err != nil {
		t.Fatal(err)
	}
	ib := &domain.Inbound{ID: uuid.NewString(), NodeID: node.ID, Tag: "t",
		Protocol: domain.ProtocolVLESS, PublicHost: "h", PublicPort: 443,
		Network: domain.NetworkTCP, Security: domain.SecurityNone}
	if err := st.CreateInbound(ctx, ib); err != nil {
		t.Fatal(err)
	}
	return &testEnv{srv: srv, store: st, apiKeys: service.NewAPIKeyService(st), inboundID: ib.ID}
}

// do issues a request with the given API key ("" sends none) and returns the
// status code and decoded JSON body (nil for empty bodies).
func (e *testEnv) do(t *testing.T, method, path, apiKey string, body any) (int, map[string]any) {
	t.Helper()
	var rd io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		rd = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, e.srv.URL+path, rd)
	if err != nil {
		t.Fatal(err)
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := e.srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatalf("non-JSON response (%d): %s", resp.StatusCode, raw)
		}
	}
	return resp.StatusCode, out
}

func errorCode(body map[string]any) string {
	e, _ := body["error"].(map[string]any)
	c, _ := e["code"].(string)
	return c
}

func (e *testEnv) createUser(t *testing.T, email string) map[string]any {
	t.Helper()
	status, body := e.do(t, "POST", "/api/v1/users", testBootstrapKey, map[string]any{
		"email": email, "inbound_ids": []string{e.inboundID},
	})
	if status != http.StatusCreated {
		t.Fatalf("create user: status %d body %v", status, body)
	}
	return body
}

func TestAuthMiddleware(t *testing.T) {
	e := newTestEnv(t)

	if status, body := e.do(t, "GET", "/api/v1/plans", "", nil); status != 401 || errorCode(body) != "unauthorized" {
		t.Errorf("no key: status %d code %q, want 401 unauthorized", status, errorCode(body))
	}
	if status, _ := e.do(t, "GET", "/api/v1/plans", "wrong-key", nil); status != 401 {
		t.Errorf("wrong key: status %d, want 401", status)
	}
	if status, _ := e.do(t, "GET", "/api/v1/plans", testBootstrapKey, nil); status != 200 {
		t.Errorf("bootstrap key: status %d, want 200", status)
	}

	// A key created through the API works; the same key revoked does not.
	created, err := e.apiKeys.Create(context.Background(), "test-"+uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	if status, _ := e.do(t, "GET", "/api/v1/plans", created.Plaintext, nil); status != 200 {
		t.Errorf("created key: status %d, want 200", status)
	}
	if err := e.apiKeys.Revoke(context.Background(), created.Key.ID); err != nil {
		t.Fatal(err)
	}
	if status, _ := e.do(t, "GET", "/api/v1/plans", created.Plaintext, nil); status != 401 {
		t.Errorf("revoked key: status %d, want 401", status)
	}

	// X-API-Key is accepted as an alternative to the Bearer header.
	req, _ := http.NewRequest("GET", e.srv.URL+"/api/v1/plans", nil)
	req.Header.Set("X-API-Key", testBootstrapKey)
	resp, err := e.srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("X-API-Key: status %d, want 200", resp.StatusCode)
	}
}

func TestUserEndpointValidation(t *testing.T) {
	e := newTestEnv(t)

	// Missing email.
	status, body := e.do(t, "POST", "/api/v1/users", testBootstrapKey, map[string]any{
		"inbound_ids": []string{e.inboundID},
	})
	if status != 400 || errorCode(body) != "validation_error" {
		t.Errorf("missing email: status %d code %q", status, errorCode(body))
	}

	// Unknown inbound id.
	status, body = e.do(t, "POST", "/api/v1/users", testBootstrapKey, map[string]any{
		"email": "v-" + uuid.NewString() + "@x.io", "inbound_ids": []string{uuid.NewString()},
	})
	if status != 400 || errorCode(body) != "validation_error" {
		t.Errorf("unknown inbound: status %d code %q", status, errorCode(body))
	}

	// Unknown JSON field.
	status, _ = e.do(t, "POST", "/api/v1/users", testBootstrapKey, map[string]any{
		"email": "v-" + uuid.NewString() + "@x.io", "inbound_ids": []string{e.inboundID}, "bogus": 1,
	})
	if status != 400 {
		t.Errorf("unknown field: status %d, want 400", status)
	}

	u := e.createUser(t, "v-"+uuid.NewString()+"@x.io")
	id := u["id"].(string)

	// Invalid status enum is rejected, valid one applied.
	status, body = e.do(t, "PATCH", "/api/v1/users/"+id, testBootstrapKey, map[string]any{"status": "bogus"})
	if status != 400 || errorCode(body) != "validation_error" {
		t.Errorf("invalid status: status %d code %q, want 400 validation_error", status, errorCode(body))
	}
	status, body = e.do(t, "PATCH", "/api/v1/users/"+id, testBootstrapKey, map[string]any{"status": "disabled"})
	if status != 200 || body["status"] != "disabled" {
		t.Errorf("valid status: status %d user status %v", status, body["status"])
	}

	// Non-integer pagination is a 400, valid pagination works.
	status, body = e.do(t, "GET", "/api/v1/users?page=abc", testBootstrapKey, nil)
	if status != 400 || errorCode(body) != "validation_error" {
		t.Errorf("page=abc: status %d code %q, want 400 validation_error", status, errorCode(body))
	}
	if status, _ = e.do(t, "GET", "/api/v1/users?per_page=xyz", testBootstrapKey, nil); status != 400 {
		t.Errorf("per_page=xyz: status %d, want 400", status)
	}
	status, body = e.do(t, "GET", "/api/v1/users?page=1&per_page=5", testBootstrapKey, nil)
	if status != 200 || body["per_page"].(float64) != 5 {
		t.Errorf("valid pagination: status %d body %v", status, body)
	}

	// Unknown user id is a 404.
	if status, _ = e.do(t, "GET", "/api/v1/users/"+uuid.NewString(), testBootstrapKey, nil); status != 404 {
		t.Errorf("unknown user: status %d, want 404", status)
	}
}

func TestUserLifecycleEndpoints(t *testing.T) {
	e := newTestEnv(t)

	u := e.createUser(t, "lc-"+uuid.NewString()+"@x.io")
	id := u["id"].(string)
	if subURL, _ := u["sub_url"].(string); !strings.HasPrefix(subURL, "http://test/sub/") {
		t.Errorf("sub_url = %v, want http://test/sub/<token>", u["sub_url"])
	}

	// Renew: 30 days out, active.
	status, body := e.do(t, "POST", "/api/v1/users/"+id+"/renew", testBootstrapKey,
		map[string]any{"days": 30})
	if status != 200 {
		t.Fatalf("renew: status %d body %v", status, body)
	}
	exp, err := time.Parse(time.RFC3339, body["expires_at"].(string))
	if err != nil {
		t.Fatalf("expires_at: %v", err)
	}
	if d := time.Until(exp).Hours() / 24; d < 29 || d > 31 {
		t.Errorf("expiry %.1f days out, want ~30", d)
	}

	// Renew with no effect requested is a validation error.
	if status, _ := e.do(t, "POST", "/api/v1/users/"+id+"/renew", testBootstrapKey,
		map[string]any{}); status != 400 {
		t.Errorf("empty renew: status %d, want 400", status)
	}

	// Suspend → disabled; resume → active.
	status, body = e.do(t, "POST", "/api/v1/users/"+id+"/suspend", testBootstrapKey, nil)
	if status != 200 || body["status"] != "disabled" {
		t.Errorf("suspend: status %d user status %v", status, body["status"])
	}
	status, body = e.do(t, "POST", "/api/v1/users/"+id+"/resume", testBootstrapKey, nil)
	if status != 200 || body["status"] != "active" {
		t.Errorf("resume: status %d user status %v", status, body["status"])
	}

	// Rotating the sub token changes the subscription URL.
	oldURL := body["sub_url"].(string)
	status, body = e.do(t, "POST", "/api/v1/users/"+id+"/rotate-sub-token", testBootstrapKey, nil)
	if status != 200 || body["sub_url"].(string) == oldURL {
		t.Errorf("rotate: status %d sub_url unchanged", status)
	}

	// Delete, then 404.
	if status, _ := e.do(t, "DELETE", "/api/v1/users/"+id, testBootstrapKey, nil); status != 204 {
		t.Errorf("delete: status %d, want 204", status)
	}
	if status, _ := e.do(t, "GET", "/api/v1/users/"+id, testBootstrapKey, nil); status != 404 {
		t.Errorf("get after delete: status %d, want 404", status)
	}
}

func signBody(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// postWebhook sends a signed (or deliberately mis-signed) billing event.
func (e *testEnv) postWebhook(t *testing.T, payload map[string]any, secret string) (int, map[string]any) {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest("POST", e.srv.URL+"/webhooks/billing", bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Webhook-Signature", signBody(secret, raw))
	resp, err := e.srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode webhook response: %v", err)
	}
	return resp.StatusCode, out
}

func TestBillingWebhookEndpoint(t *testing.T) {
	e := newTestEnv(t)
	extID := "sub_" + uuid.NewString()

	// Wrong secret → 401 before any processing.
	status, body := e.postWebhook(t, map[string]any{
		"event_id": "evt_" + uuid.NewString(), "action": "suspend", "external_id": extID,
	}, "wrong-secret")
	if status != 401 || errorCode(body) != "invalid_signature" {
		t.Fatalf("bad signature: status %d code %q", status, errorCode(body))
	}

	// Unknown action → 400.
	status, _ = e.postWebhook(t, map[string]any{
		"event_id": "evt_" + uuid.NewString(), "action": "explode",
	}, testBillingKey)
	if status != 400 {
		t.Fatalf("unknown action: status %d, want 400", status)
	}

	// A renew for a user that doesn't exist yet fails with 404 — and must
	// NOT consume the event id: the provider's retry has to be processed,
	// not answered as a duplicate.
	evtRenew := "evt_" + uuid.NewString()
	renew := map[string]any{
		"event_id": evtRenew, "action": "renew", "external_id": extID,
		"renew": map[string]any{"days": 30},
	}
	if status, _ := e.postWebhook(t, renew, testBillingKey); status != 404 {
		t.Fatalf("renew before provision: status %d, want 404", status)
	}
	status, body = e.postWebhook(t, renew, testBillingKey)
	if status == 200 && body["duplicate"] == true {
		t.Fatal("failed event was recorded as processed; retry swallowed as duplicate")
	}
	if status != 404 {
		t.Fatalf("renew retry: status %d, want 404", status)
	}

	// Provision the user, then the same renew event id succeeds.
	evtProv := "evt_" + uuid.NewString()
	prov := map[string]any{
		"event_id": evtProv, "action": "provision", "external_id": extID,
		"provision": map[string]any{
			"email":       "wh-" + uuid.NewString() + "@x.io",
			"inbound_ids": []string{e.inboundID},
		},
	}
	status, body = e.postWebhook(t, prov, testBillingKey)
	if status != 200 || body["duplicate"] != false || body["user"] == nil {
		t.Fatalf("provision: status %d body %v", status, body)
	}
	status, body = e.postWebhook(t, renew, testBillingKey)
	if status != 200 || body["duplicate"] != false {
		t.Fatalf("renew after provision: status %d body %v", status, body)
	}

	// Replaying a successfully processed event reports duplicate.
	status, body = e.postWebhook(t, prov, testBillingKey)
	if status != 200 || body["duplicate"] != true {
		t.Fatalf("replay: status %d body %v", status, body)
	}
	if body["user"] != nil {
		t.Error("duplicate response should not re-execute and return a user")
	}
}

func TestSubscriptionEndpoint(t *testing.T) {
	e := newTestEnv(t)

	u := e.createUser(t, "sub-"+uuid.NewString()+"@x.io")
	subURL := u["sub_url"].(string)
	token := subURL[strings.LastIndex(subURL, "/")+1:]

	resp, err := e.srv.Client().Get(e.srv.URL + "/sub/" + token)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("subscription: status %d, want 200", resp.StatusCode)
	}
	if resp.Header.Get("Subscription-Userinfo") == "" {
		t.Error("missing Subscription-Userinfo header")
	}
	if raw, _ := io.ReadAll(resp.Body); len(raw) == 0 {
		t.Error("empty subscription body")
	}

	// Info endpoint exposes usage as JSON.
	status, body := e.do(t, "GET", fmt.Sprintf("/sub/%s/info", token), "", nil)
	if status != 200 || body["email"] != u["email"] {
		t.Errorf("sub info: status %d body %v", status, body)
	}

	// Unknown tokens are a uniform 404.
	if status, _ := e.do(t, "GET", "/sub/"+uuid.NewString(), "", nil); status != 404 {
		t.Errorf("unknown token: status %d, want 404", status)
	}
}
