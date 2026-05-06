package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Andamio-Platform/andamio-cli/internal/apierr"
	"github.com/Andamio-Platform/andamio-cli/internal/config"
	"github.com/Andamio-Platform/andamio-cli/internal/output"
)

// =============================================================================
// dev keys list|create|delete — andamio-api /v2/keys (#410, PR-B/#80 second slice)
// =============================================================================

// devKeysGatewayStub stands in for the /v2/keys endpoint family. Each test
// wires the response body + status and inspects captured request metadata
// (method, body, Authorization header) so the no-X-API-Key contract is
// enforced structurally — not just observed transitively.
type devKeysGatewayStub struct {
	t *testing.T

	listRespStatus int
	listRespBody   []byte
	gotListRequest bool

	createRespStatus int
	createRespBody   []byte
	gotCreateRequest bool
	capturedCreate   map[string]interface{}

	deleteRespStatus int
	deleteRespBody   []byte
	gotDeleteRequest bool
	capturedDeleteID string

	// capturedAuthHeader records the Authorization header from the last
	// request. Used to assert the dev JWT (NOT the wallet JWT, NOT the
	// api key) was forwarded.
	capturedAuthHeader string
	// capturedAPIKeyHeader records X-API-Key from the last request. Must
	// always be empty on this endpoint family — dual-credential requests
	// fail with the gateway's middleware.
	capturedAPIKeyHeader string
}

func (s *devKeysGatewayStub) serve() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/keys", func(w http.ResponseWriter, r *http.Request) {
		s.capturedAuthHeader = r.Header.Get("Authorization")
		s.capturedAPIKeyHeader = r.Header.Get("X-API-Key")
		switch r.Method {
		case http.MethodGet:
			s.gotListRequest = true
			s.writeOrDefault(w, s.listRespStatus, s.listRespBody)
		case http.MethodPost:
			s.gotCreateRequest = true
			_ = json.NewDecoder(r.Body).Decode(&s.capturedCreate)
			s.writeOrDefault(w, s.createRespStatus, s.createRespBody)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	// Per-id delete — match anything under /api/v2/keys/.
	mux.HandleFunc("/api/v2/keys/", func(w http.ResponseWriter, r *http.Request) {
		s.capturedAuthHeader = r.Header.Get("Authorization")
		s.capturedAPIKeyHeader = r.Header.Get("X-API-Key")
		if r.Method != http.MethodDelete {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.gotDeleteRequest = true
		s.capturedDeleteID = strings.TrimPrefix(r.URL.Path, "/api/v2/keys/")
		// Default to 204 No Content on delete (the spec says 204; tests
		// override deleteRespStatus to exercise error paths).
		status := s.deleteRespStatus
		if status == 0 {
			status = http.StatusNoContent
		}
		w.WriteHeader(status)
		_, _ = w.Write(s.deleteRespBody)
	})
	return mux
}

func (s *devKeysGatewayStub) writeOrDefault(w http.ResponseWriter, status int, body []byte) {
	if status == 0 {
		status = http.StatusOK
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

// devKeysTestEnv wires the stub, points HOME at a tempdir, and seeds a
// config with both a dev JWT (the credential `dev keys` SHOULD use) and a
// wallet JWT + api key (credentials it MUST NOT also send). The latter two
// are tripwires: any test that sees them on the wire fails.
func devKeysTestEnv(t *testing.T, stub *devKeysGatewayStub) *config.Config {
	t.Helper()
	stub.t = t
	srv := httptest.NewServer(stub.serve())
	t.Cleanup(srv.Close)

	t.Setenv("HOME", t.TempDir())

	cfg := &config.Config{
		BaseURL:   srv.URL,
		APIKey:    "tripwire-api-key-MUST-NOT-LEAK",
		UserJWT:   "tripwire-user-jwt-MUST-NOT-LEAK",
		DevJWT:    "dev.jwt.bearer.value",
		DevAlias:  "myalias",
		DevID:     "dev-1",
		DevTier:   "pioneer",
	}
	if err := config.Save(cfg); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	return cfg
}

// -----------------------------------------------------------------------------
// shared auth-header invariants (one assertion path used by all three commands)
// -----------------------------------------------------------------------------

func assertOnlyDevJWTOnTheWire(t *testing.T, stub *devKeysGatewayStub) {
	t.Helper()
	if got, want := stub.capturedAuthHeader, "Bearer dev.jwt.bearer.value"; got != want {
		t.Errorf("Authorization header = %q, want %q (dev JWT must ride here, not wallet JWT)", got, want)
	}
	if stub.capturedAPIKeyHeader != "" {
		t.Errorf("X-API-Key header = %q, want empty (dual-credential requests are rejected by developerJWTAuth middleware)", stub.capturedAPIKeyHeader)
	}
}

// -----------------------------------------------------------------------------
// dev keys list
// -----------------------------------------------------------------------------

func TestRunDevKeysList_HappyPath_DecodesAndPassesAuthHeaderCorrectly(t *testing.T) {
	stub := &devKeysGatewayStub{
		listRespBody: []byte(`{"keys":[
            {"id":"k1","name":"preprod-bot","environment":"preprod","last4":"abcd","created_at":"2026-04-01T00:00:00Z"},
            {"id":"k2","name":"mainnet-prod","environment":"mainnet","last4":"","created_at":"2026-03-01T00:00:00Z"}
        ]}`),
	}
	cfg := devKeysTestEnv(t, stub)

	if err := runDevKeysListFlow(context.Background(), cfg); err != nil {
		t.Fatalf("list: %v", err)
	}
	if !stub.gotListRequest {
		t.Fatal("list endpoint not called")
	}
	assertOnlyDevJWTOnTheWire(t, stub)
}

func TestRunDevKeysList_JSONOutput_PassesGatewayShapeVerbatim(t *testing.T) {
	stub := &devKeysGatewayStub{
		listRespBody: []byte(`{"keys":[{"id":"k1","name":"bot","environment":"preprod","last4":"abcd","created_at":"2026-04-01T00:00:00Z"}]}`),
	}
	cfg := devKeysTestEnv(t, stub)

	captured := captureStdout(t, func() {
		_ = output.SetFormat("json")
		t.Cleanup(func() { _ = output.SetFormat("text") })
		if err := runDevKeysListFlow(context.Background(), cfg); err != nil {
			t.Fatalf("list: %v", err)
		}
	})

	var got map[string]interface{}
	if err := json.Unmarshal([]byte(captured), &got); err != nil {
		t.Fatalf("decode list JSON: %v\nbytes: %s", err, captured)
	}
	keys, ok := got["keys"].([]interface{})
	if !ok || len(keys) != 1 {
		t.Fatalf("envelope.keys missing or wrong length: %v", got)
	}
	k := keys[0].(map[string]interface{})
	for field, want := range map[string]interface{}{
		"id":          "k1",
		"name":        "bot",
		"environment": "preprod",
		"last4":       "abcd",
	} {
		if k[field] != want {
			t.Errorf("envelope.keys[0].%s = %v, want %v", field, k[field], want)
		}
	}
}

func TestRunDevKeysList_NoDevAuth_ErrorsWithLoginHint(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg := &config.Config{}
	if err := config.Save(cfg); err != nil {
		t.Fatalf("seed empty: %v", err)
	}

	err := runDevKeysListFlow(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected AuthError when dev slot is empty")
	}
	var authErr *apierr.AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("expected *apierr.AuthError, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "andamio dev login") {
		t.Errorf("err = %q, want substring %q (the user-facing remediation)", err.Error(), "andamio dev login")
	}
}

// -----------------------------------------------------------------------------
// dev keys create
// -----------------------------------------------------------------------------

func TestRunDevKeysCreate_HappyPath_RawKeyOnStdoutWarningOnStderr(t *testing.T) {
	stub := &devKeysGatewayStub{
		createRespBody: []byte(`{"id":"k-new","name":"preprod-bot","environment":"preprod","key":"ak_test_RAWVALUE","last4":"alue","created_at":"2026-05-01T00:00:00Z"}`),
	}
	cfg := devKeysTestEnv(t, stub)

	captured := captureStdout(t, func() {
		if err := runDevKeysCreateFlow(context.Background(), cfg, "preprod-bot", "preprod"); err != nil {
			t.Fatalf("create: %v", err)
		}
	})

	if !stub.gotCreateRequest {
		t.Fatal("create endpoint not called")
	}
	assertOnlyDevJWTOnTheWire(t, stub)

	// Body shape: name + environment forwarded verbatim.
	if got := stub.capturedCreate["name"]; got != "preprod-bot" {
		t.Errorf("create body name = %v, want preprod-bot", got)
	}
	if got := stub.capturedCreate["environment"]; got != "preprod" {
		t.Errorf("create body environment = %v, want preprod", got)
	}

	// Stdout MUST contain the raw key (so it can be piped/captured).
	if !strings.Contains(captured, "ak_test_RAWVALUE") {
		t.Errorf("stdout missing raw key; captured: %q", captured)
	}
	// Stdout MUST NOT contain the metadata noise — that goes to stderr,
	// so a `dev keys create … | pbcopy` workflow gets the key alone.
	if strings.Contains(captured, "WARNING") || strings.Contains(captured, "id: k-new") {
		t.Errorf("stdout polluted with metadata (must go to stderr); captured: %q", captured)
	}
}

func TestRunDevKeysCreate_JSONOutput_KeyValuePresent(t *testing.T) {
	// JSON mode is the scripting surface — the raw key MUST appear in the
	// envelope (not as an absence) so an agent can capture it programmatically.
	// This deliberately differs from `dev login`/`dev refresh` which never
	// surface tokens on stdout: there, the JWT is persisted to disk and the
	// CLI reads it back; here, the gateway returns the raw key exactly once
	// and the CLI cannot recover it later. JSON output is the user's
	// only structured path to capture it.
	stub := &devKeysGatewayStub{
		createRespBody: []byte(`{"id":"k-new","name":"a","environment":"mainnet","key":"ak_RAW","last4":"k_RAW","created_at":"2026-05-01T00:00:00Z"}`),
	}
	cfg := devKeysTestEnv(t, stub)

	captured := captureStdout(t, func() {
		_ = output.SetFormat("json")
		t.Cleanup(func() { _ = output.SetFormat("text") })
		if err := runDevKeysCreateFlow(context.Background(), cfg, "a", "mainnet"); err != nil {
			t.Fatalf("create: %v", err)
		}
	})

	var got map[string]interface{}
	if err := json.Unmarshal([]byte(captured), &got); err != nil {
		t.Fatalf("decode: %v\nbytes: %s", err, captured)
	}
	if got["key"] != "ak_RAW" {
		t.Errorf("envelope.key = %v, want ak_RAW", got["key"])
	}
}

func TestRunDevKeysCreate_GatewayReturnsEmptyKey_RefusesToSilentlySucceed(t *testing.T) {
	// A 200 with an empty `key` field means the raw value is unrecoverable —
	// the developer would have a key id they cannot use. Refuse rather than
	// pretend success.
	stub := &devKeysGatewayStub{
		createRespBody: []byte(`{"id":"k-x","name":"a","environment":"mainnet","key":"","last4":"","created_at":"2026-05-01T00:00:00Z"}`),
	}
	cfg := devKeysTestEnv(t, stub)

	err := runDevKeysCreateFlow(context.Background(), cfg, "a", "mainnet")
	if err == nil {
		t.Fatal("expected error when gateway returns empty key (raw value unrecoverable)")
	}
	if !strings.Contains(err.Error(), "no key value") {
		t.Errorf("err = %q, want substring %q", err.Error(), "no key value")
	}
}

func TestRunDevKeysCreate_TierLimitExceededBubbles(t *testing.T) {
	// 429 with the gateway's stable error code — the CLI must surface the
	// code so scripts can branch on it.
	stub := &devKeysGatewayStub{
		createRespStatus: http.StatusTooManyRequests,
		createRespBody:   []byte(`{"error":"tier_limit_exceeded","message":"tier cap reached"}`),
	}
	cfg := devKeysTestEnv(t, stub)

	err := runDevKeysCreateFlow(context.Background(), cfg, "a", "mainnet")
	if err == nil {
		t.Fatal("expected error on 429 tier-limit response")
	}
	if !strings.Contains(err.Error(), "tier_limit_exceeded") {
		t.Errorf("err = %q, want substring %q (gateway's stable code must reach the user)", err.Error(), "tier_limit_exceeded")
	}
}

// -----------------------------------------------------------------------------
// dev keys delete
// -----------------------------------------------------------------------------

func TestRunDevKeysDelete_HappyPath_204(t *testing.T) {
	stub := &devKeysGatewayStub{
		deleteRespStatus: http.StatusNoContent,
	}
	cfg := devKeysTestEnv(t, stub)

	if err := runDevKeysDeleteFlow(context.Background(), cfg, "key-uuid-1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if !stub.gotDeleteRequest {
		t.Fatal("delete endpoint not called")
	}
	if got := stub.capturedDeleteID; got != "key-uuid-1" {
		t.Errorf("captured id = %q, want key-uuid-1", got)
	}
	assertOnlyDevJWTOnTheWire(t, stub)
}

func TestRunDevKeysDelete_NotFound_DocumentsAmbiguity(t *testing.T) {
	// Per the gateway's threat model, 404 means BOTH "no such id" and "id
	// belongs to another developer" — indistinguishable on the wire. The
	// CLI's error message must name both possibilities so the user does
	// not waste time debugging a typo when the actual problem is they're
	// trying to revoke a key they don't own.
	stub := &devKeysGatewayStub{
		deleteRespStatus: http.StatusNotFound,
		deleteRespBody:   []byte(`{"error":"not found"}`),
	}
	cfg := devKeysTestEnv(t, stub)

	err := runDevKeysDeleteFlow(context.Background(), cfg, "missing-id")
	if err == nil {
		t.Fatal("expected error on 404")
	}
	for _, want := range []string{"missing-id", "not owned"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("err = %q, want substring %q", err.Error(), want)
		}
	}
}

func TestRunDevKeysDelete_JSONEnvelope(t *testing.T) {
	stub := &devKeysGatewayStub{
		deleteRespStatus: http.StatusNoContent,
	}
	cfg := devKeysTestEnv(t, stub)

	captured := captureStdout(t, func() {
		_ = output.SetFormat("json")
		t.Cleanup(func() { _ = output.SetFormat("text") })
		if err := runDevKeysDeleteFlow(context.Background(), cfg, "k-1"); err != nil {
			t.Fatalf("delete: %v", err)
		}
	})

	var got map[string]interface{}
	if err := json.Unmarshal([]byte(captured), &got); err != nil {
		t.Fatalf("decode: %v\nbytes: %s", err, captured)
	}
	if got["id"] != "k-1" {
		t.Errorf("envelope.id = %v, want k-1", got["id"])
	}
	if v, _ := got["deleted"].(bool); !v {
		t.Errorf("envelope.deleted = %v, want true", got["deleted"])
	}
}

// -----------------------------------------------------------------------------
// devKeysClient — the auth-isolation primitive (the load-bearing security guard)
// -----------------------------------------------------------------------------

func TestDevKeysClient_StripsAPIKeyAndPromotesDevJWT(t *testing.T) {
	cfg := &config.Config{
		APIKey:  "should-be-stripped",
		UserJWT: "wallet-jwt-should-not-promote",
		DevJWT:  "dev-jwt-should-be-bearer",
	}
	c, err := devKeysClient(cfg)
	if err != nil {
		t.Fatalf("devKeysClient: %v", err)
	}
	if c == nil {
		t.Fatal("nil client")
	}
	// Roundtrip via httptest to verify on-the-wire headers — direct field
	// access on the unexported client struct would couple to internals.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.Header.Get("Authorization"), "Bearer dev-jwt-should-be-bearer"; got != want {
			t.Errorf("Authorization = %q, want %q", got, want)
		}
		if got := r.Header.Get("X-API-Key"); got != "" {
			t.Errorf("X-API-Key = %q, want empty (dual-credential requests are rejected)", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"keys":[]}`))
	}))
	t.Cleanup(srv.Close)
	cfg.BaseURL = srv.URL
	c, _ = devKeysClient(cfg)

	var resp devKeysListResponse
	if err := c.Get(context.Background(), devKeysListPath, &resp); err != nil {
		t.Fatalf("get: %v", err)
	}

	// Critical: the original cfg must not be mutated by devKeysClient.
	// Subsequent config.Save() should write the unchanged state back.
	if cfg.APIKey != "should-be-stripped" || cfg.UserJWT != "wallet-jwt-should-not-promote" {
		t.Errorf("devKeysClient mutated source cfg: APIKey=%q UserJWT=%q (want unchanged)", cfg.APIKey, cfg.UserJWT)
	}
}
