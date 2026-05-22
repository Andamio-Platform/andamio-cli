package main

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Andamio-Platform/andamio-cli/internal/apierr"
	"github.com/Andamio-Platform/andamio-cli/internal/config"
	"github.com/Andamio-Platform/andamio-cli/internal/output"
	"github.com/spf13/cobra"
)

// =============================================================================
// dev login / refresh — CIP-30 happy paths and error coverage (andamio-api #410)
// =============================================================================

// devGatewayStub stands in for the v2.3 secure-developer-login surface.
// Each test wires the stub bodies + status codes and inspects the captured
// request payloads to assert on what the CLI actually sent. Three endpoints
// are routed; unmatched paths 404 so a path-rename regression in production
// code shows up as a clean test failure rather than a vague decode error.
type devGatewayStub struct {
	t *testing.T

	sessionRespStatus int
	sessionRespBody   []byte
	gotSessionRequest bool
	capturedSession   map[string]interface{}

	completeRespStatus int
	completeRespBody   []byte
	gotCompleteRequest bool
	capturedComplete   map[string]interface{}

	refreshRespStatus int
	refreshRespBody   []byte
	gotRefreshRequest bool
	capturedRefresh   map[string]interface{}
}

func (s *devGatewayStub) serve() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/auth/developer/login/session", func(w http.ResponseWriter, r *http.Request) {
		s.gotSessionRequest = true
		_ = json.NewDecoder(r.Body).Decode(&s.capturedSession)
		s.writeOrDefault(w, s.sessionRespStatus, s.sessionRespBody)
	})
	mux.HandleFunc("/api/v2/auth/developer/login/complete", func(w http.ResponseWriter, r *http.Request) {
		s.gotCompleteRequest = true
		_ = json.NewDecoder(r.Body).Decode(&s.capturedComplete)
		s.writeOrDefault(w, s.completeRespStatus, s.completeRespBody)
	})
	mux.HandleFunc("/api/v2/auth/developer/token/refresh", func(w http.ResponseWriter, r *http.Request) {
		s.gotRefreshRequest = true
		_ = json.NewDecoder(r.Body).Decode(&s.capturedRefresh)
		s.writeOrDefault(w, s.refreshRespStatus, s.refreshRespBody)
	})
	return mux
}

func (s *devGatewayStub) writeOrDefault(w http.ResponseWriter, status int, body []byte) {
	if status == 0 {
		status = http.StatusOK
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

// devTestEnv wires a stubbed gateway, an isolated $HOME so config.Save() does
// not clobber the developer's real ~/.andamio/config.json, and an ephemeral
// ed25519 keypair (so cardano.SignMessage actually runs). Returns the cfg the
// caller hands to the runDev* helpers plus the keys.
func devTestEnv(t *testing.T, stub *devGatewayStub) (*config.Config, ed25519.PrivateKey, ed25519.PublicKey) {
	t.Helper()
	stub.t = t
	srv := httptest.NewServer(stub.serve())
	t.Cleanup(srv.Close)

	// Sandbox config.Save: it writes to filepath.Join(os.UserHomeDir(),
	// ".andamio/config.json"). os.UserHomeDir() reads $HOME on darwin/linux,
	// so pointing $HOME at a tempdir keeps the test off the real config.
	t.Setenv("HOME", t.TempDir())

	cfg := &config.Config{BaseURL: srv.URL}

	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate ed25519 key: %v", err)
	}
	return cfg, priv, pub
}

// secureLoginBody returns the canonical SecureLoginResponse JSON for tests.
// Keeps each test from re-encoding the same nested shape five times.
func secureLoginBody(jwt, refresh, alias, userID, tier, jwtExp, refreshExp string) []byte {
	body, _ := json.Marshal(map[string]interface{}{
		"user_id": userID,
		"alias":   alias,
		"tier":    tier,
		"jwt": map[string]string{
			"token":      jwt,
			"expires_at": jwtExp,
		},
		"refresh_token": map[string]string{
			"token":      refresh,
			"expires_at": refreshExp,
		},
	})
	return body
}

// -----------------------------------------------------------------------------
// dev login (CIP-30)
// -----------------------------------------------------------------------------

func TestRunDevHeadlessLogin_HappyPath_PersistsAllSlots(t *testing.T) {
	stub := &devGatewayStub{
		sessionRespBody: []byte(`{"session_id":"sess-uuid","nonce":"please-sign-this","expires_at":"2099-01-01T00:05:00Z"}`),
		completeRespBody: secureLoginBody(
			"jwt.token.aaa",
			"refresh.token.bbb",
			"myalias",
			"dev-user-1",
			"pioneer",
			"2099-01-01T01:00:00Z",
			"2099-02-01T00:00:00Z",
		),
	}
	cfg, priv, pub := devTestEnv(t, stub)

	if err := runDevHeadlessLogin(context.Background(), cfg, priv, pub, "ignored.skey", "myalias", "addr_test1xyz"); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// Both endpoints called; refresh not called by login flow.
	if !stub.gotSessionRequest {
		t.Errorf("session endpoint was not called")
	}
	if !stub.gotCompleteRequest {
		t.Errorf("complete endpoint was not called")
	}
	if stub.gotRefreshRequest {
		t.Errorf("login flow must not hit token/refresh")
	}

	// Session body must carry alias + wallet_address (#410 keying).
	if got, want := stub.capturedSession["alias"], "myalias"; got != want {
		t.Errorf("session body alias = %v, want %v", got, want)
	}
	if got, want := stub.capturedSession["wallet_address"], "addr_test1xyz"; got != want {
		t.Errorf("session body wallet_address = %v, want %v", got, want)
	}

	// Complete body: session_id + signature only — alias/address bind at
	// session creation, not here.
	if got, want := stub.capturedComplete["session_id"], "sess-uuid"; got != want {
		t.Errorf("complete body session_id = %v, want %v", got, want)
	}
	if _, present := stub.capturedComplete["alias"]; present {
		t.Errorf("complete body must not echo alias (binds at session, not complete)")
	}
	if _, present := stub.capturedComplete["wallet_address"]; present {
		t.Errorf("complete body must not echo wallet_address")
	}
	sig, ok := stub.capturedComplete["signature"].(map[string]interface{})
	if !ok {
		t.Fatalf("complete body signature not an object: %T", stub.capturedComplete["signature"])
	}
	if sig["signature"] == "" || sig["key"] == "" {
		t.Errorf("complete body signature.{signature,key} must be populated; got %v", sig)
	}

	// All four slots persisted (JWT, refresh, alias/id, tier).
	if got, want := cfg.DevJWT, "jwt.token.aaa"; got != want {
		t.Errorf("cfg.DevJWT = %q, want %q", got, want)
	}
	if got, want := cfg.DevJWTExpiresAt, "2099-01-01T01:00:00Z"; got != want {
		t.Errorf("cfg.DevJWTExpiresAt = %q, want %q", got, want)
	}
	if got, want := cfg.DevRefreshToken, "refresh.token.bbb"; got != want {
		t.Errorf("cfg.DevRefreshToken = %q, want %q", got, want)
	}
	if got, want := cfg.DevRefreshTokenExpiresAt, "2099-02-01T00:00:00Z"; got != want {
		t.Errorf("cfg.DevRefreshTokenExpiresAt = %q, want %q", got, want)
	}
	if got, want := cfg.DevAlias, "myalias"; got != want {
		t.Errorf("cfg.DevAlias = %q, want %q", got, want)
	}
	if got, want := cfg.DevID, "dev-user-1"; got != want {
		t.Errorf("cfg.DevID = %q, want %q", got, want)
	}
	if got, want := cfg.DevTier, "pioneer"; got != want {
		t.Errorf("cfg.DevTier = %q, want %q", got, want)
	}
	if cfg.DevKeyHash == "" {
		t.Errorf("cfg.DevKeyHash empty; cardano.SignMessage should have computed a Blake2b-224 hex hash")
	}

	// Disk read-back: prove config.Save wrote what the in-memory cfg has.
	loaded, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load after dev login: %v", err)
	}
	if loaded.DevJWT != cfg.DevJWT || loaded.DevRefreshToken != cfg.DevRefreshToken {
		t.Errorf("on-disk config drifted from in-memory cfg\n on-disk:  %+v\n in-mem:   %+v", loaded, cfg)
	}
}

// TestRunDevHeadlessLogin_JSONOutputShape pins the --output json envelope
// for `dev login`: keys agents will read, alignment with `dev status`, and
// — critically — that the JWT and refresh-token bodies the CLI just
// persisted to ~/.andamio/config.json never appear on stdout. Tokens belong
// on disk, not in shell histories or CI logs.
func TestRunDevHeadlessLogin_JSONOutputShape(t *testing.T) {
	stub := &devGatewayStub{
		sessionRespBody: []byte(`{"session_id":"sess-uuid","nonce":"please-sign-this","expires_at":"2099-01-01T00:05:00Z"}`),
		completeRespBody: secureLoginBody(
			"jwt.SECRET.SHOULD-NOT-LEAK",
			"refresh.SECRET.SHOULD-NOT-LEAK",
			"myalias",
			"dev-user-1",
			"pioneer",
			"2099-01-01T01:00:00Z",
			"2099-02-01T00:00:00Z",
		),
	}
	cfg, priv, pub := devTestEnv(t, stub)

	captured := captureStdout(t, func() {
		_ = output.SetFormat("json")
		t.Cleanup(func() { _ = output.SetFormat("text") })
		if err := runDevHeadlessLogin(context.Background(), cfg, priv, pub, "ignored.skey", "myalias", "addr_test1xyz"); err != nil {
			t.Fatalf("login: %v", err)
		}
	})

	// Security guard: token bodies must never reach stdout. captureStdout
	// reads only os.Stdout, so anything routed correctly to os.Stderr stays
	// out of `captured`.
	if strings.Contains(captured, "SECRET.SHOULD-NOT-LEAK") {
		t.Fatalf("token body leaked to stdout — JSON envelope must NEVER carry the JWT or refresh token. Captured bytes:\n%s", captured)
	}

	var got map[string]interface{}
	if err := json.Unmarshal([]byte(captured), &got); err != nil {
		t.Fatalf("decode dev login JSON: %v\nbytes: %s", err, captured)
	}

	// Key set: must match dev status / dev refresh on the shared keys.
	for k, want := range map[string]interface{}{
		"alias":                    "myalias",
		"dev_id":                   "dev-user-1",
		"tier":                     "pioneer",
		"jwt_expires_at":           "2099-01-01T01:00:00Z",
		"refresh_token_expires_at": "2099-02-01T00:00:00Z",
	} {
		if got[k] != want {
			t.Errorf("envelope[%q] = %v, want %v", k, got[k], want)
		}
	}
	// key_hash is login-only (refresh does not re-sign). Must be present
	// AND non-empty here so agents can pin the signing key for audit.
	if kh, _ := got["key_hash"].(string); kh == "" {
		t.Errorf("envelope is missing key_hash on login output — present-and-nonempty is the contract for the login envelope")
	}
	// Token-shaped keys MUST NOT exist in the envelope.
	for _, k := range []string{"jwt", "dev_jwt", "refresh_token", "dev_refresh_token"} {
		if _, present := got[k]; present {
			t.Errorf("envelope must not include %q — token bodies stay on disk only", k)
		}
	}
	// Cross-command consistency: assert NOT the legacy `refresh_expires_at`
	// (the pre-fix name) so a future revert is caught.
	if _, present := got["refresh_expires_at"]; present {
		t.Errorf("envelope still uses legacy `refresh_expires_at` key — should be `refresh_token_expires_at` to align with `dev status`")
	}
}

func TestRunDevHeadlessLogin_FallsBackToFlagAliasWhenResponseEmpty(t *testing.T) {
	// The gateway echoes a populated alias on success, but the CLI must defend
	// against a future shape where it's omitted (omitempty) — the flag value
	// is the safest fallback since the user just typed it.
	stub := &devGatewayStub{
		sessionRespBody: []byte(`{"session_id":"sess-x","nonce":"n","expires_at":""}`),
		completeRespBody: secureLoginBody(
			"jwt.x", "refresh.x", "" /* alias omitted */, "dev-1", "pioneer",
			"2099-01-01T01:00:00Z", "2099-02-01T00:00:00Z",
		),
	}
	cfg, priv, pub := devTestEnv(t, stub)

	if err := runDevHeadlessLogin(context.Background(), cfg, priv, pub, "x", "fallback-alias", "addr"); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got, want := cfg.DevAlias, "fallback-alias"; got != want {
		t.Errorf("cfg.DevAlias = %q, want %q (flag fallback)", got, want)
	}
}

func TestRunDevHeadlessLogin_GatewayCanonicalAliasOverridesFlag(t *testing.T) {
	stub := &devGatewayStub{
		sessionRespBody: []byte(`{"session_id":"sess-x","nonce":"n"}`),
		completeRespBody: secureLoginBody(
			"jwt.x", "refresh.x", "canonical-alias", "dev-1", "pioneer",
			"2099-01-01T01:00:00Z", "2099-02-01T00:00:00Z",
		),
	}
	cfg, priv, pub := devTestEnv(t, stub)

	if err := runDevHeadlessLogin(context.Background(), cfg, priv, pub, "x", "stale-flag", "addr"); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got, want := cfg.DevAlias, "canonical-alias"; got != want {
		t.Errorf("cfg.DevAlias = %q, want %q (server canonical)", got, want)
	}
}

func TestRunDevHeadlessLogin_SessionMissingNonceErrors(t *testing.T) {
	stub := &devGatewayStub{
		sessionRespBody:  []byte(`{"session_id":"sess-x","nonce":"","expires_at":""}`),
		completeRespBody: []byte(`unreachable`),
	}
	cfg, priv, pub := devTestEnv(t, stub)

	err := runDevHeadlessLogin(context.Background(), cfg, priv, pub, "x", "myalias", "addr")
	if err == nil {
		t.Fatal("expected error when session response has empty nonce")
	}
	if !strings.Contains(err.Error(), "missing nonce or session_id") {
		t.Errorf("err = %q, want substring %q", err.Error(), "missing nonce or session_id")
	}
	if stub.gotCompleteRequest {
		t.Errorf("complete endpoint should not be reached when session is invalid")
	}
	if cfg.DevJWT != "" || cfg.DevRefreshToken != "" {
		t.Errorf("config slots should remain empty on session-error path; got jwt=%q refresh=%q", cfg.DevJWT, cfg.DevRefreshToken)
	}
}

func TestRunDevHeadlessLogin_CompleteMissingJWTErrors(t *testing.T) {
	stub := &devGatewayStub{
		sessionRespBody:  []byte(`{"session_id":"sess-x","nonce":"n"}`),
		completeRespBody: secureLoginBody("" /* no jwt */, "refresh.x", "a", "u", "pioneer", "", ""),
	}
	cfg, priv, pub := devTestEnv(t, stub)

	err := runDevHeadlessLogin(context.Background(), cfg, priv, pub, "x", "myalias", "addr")
	if err == nil {
		t.Fatal("expected error when complete response has empty jwt")
	}
	if !strings.Contains(err.Error(), "no JWT received") {
		t.Errorf("err = %q, want substring %q", err.Error(), "no JWT received")
	}
}

func TestRunDevHeadlessLogin_CompleteMissingRefreshTokenErrors(t *testing.T) {
	// Refusing to persist a session without a refresh token prevents a future
	// `dev refresh` from blaming the user for the gateway's omission.
	stub := &devGatewayStub{
		sessionRespBody:  []byte(`{"session_id":"sess-x","nonce":"n"}`),
		completeRespBody: secureLoginBody("jwt.x", "" /* no refresh */, "a", "u", "pioneer", "", ""),
	}
	cfg, priv, pub := devTestEnv(t, stub)

	err := runDevHeadlessLogin(context.Background(), cfg, priv, pub, "x", "myalias", "addr")
	if err == nil {
		t.Fatal("expected error when complete response has empty refresh_token")
	}
	if !strings.Contains(err.Error(), "no refresh token") {
		t.Errorf("err = %q, want substring %q", err.Error(), "no refresh token")
	}
	if cfg.DevJWT != "" {
		t.Errorf("partial persistence: cfg.DevJWT was written before the refresh-token check; got %q", cfg.DevJWT)
	}
}

// TestRunDevHeadlessLogin_SessionExpiredDuringSigningEmitsClearError pins
// the pre-check at runDevHeadlessLogin step 2b: when the gateway returned
// a session with an already-past expires_at, the CLI must NOT call /complete
// (which would 401 with the misleading address/.skey hint). Instead, surface
// the actual cause: signing took longer than the 5-min server window.
func TestRunDevHeadlessLogin_SessionExpiredDuringSigningEmitsClearError(t *testing.T) {
	stub := &devGatewayStub{
		// Year-1 timestamp serializes from a zero time.Time and is reliably
		// in the past regardless of system clock skew.
		sessionRespBody:  []byte(`{"session_id":"sess-x","nonce":"n","expires_at":"0001-01-01T00:00:00Z"}`),
		completeRespBody: []byte(`unreachable`),
	}
	cfg, priv, pub := devTestEnv(t, stub)

	err := runDevHeadlessLogin(context.Background(), cfg, priv, pub, "x", "myalias", "addr")
	if err == nil {
		t.Fatal("expected error when session expired during signing")
	}
	if !strings.Contains(err.Error(), "session expired during signing") {
		t.Errorf("err = %q, want substring %q (the user-facing diagnostic)", err.Error(), "session expired during signing")
	}
	if stub.gotCompleteRequest {
		t.Errorf("complete endpoint must NOT be called after a locally-detected expired session — the whole point is to skip the gateway 401 + misleading hint")
	}
}

func TestRunDevHeadlessLogin_CompleteAuthErrorBubblesAsTypedAuthError(t *testing.T) {
	stub := &devGatewayStub{
		sessionRespBody:    []byte(`{"session_id":"sess-x","nonce":"n"}`),
		completeRespStatus: http.StatusUnauthorized,
		completeRespBody:   []byte(`{"error":"signature did not verify"}`),
	}
	cfg, priv, pub := devTestEnv(t, stub)

	err := runDevHeadlessLogin(context.Background(), cfg, priv, pub, "x", "myalias", "addr")
	if err == nil {
		t.Fatal("expected error on 401 from complete")
	}
	var authErr *apierr.AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("expected *apierr.AuthError to bubble through, got %T: %v", err, err)
	}
	if authErr.HTTPStatus != http.StatusUnauthorized {
		t.Errorf("HTTPStatus = %d, want 401", authErr.HTTPStatus)
	}
	// 401 from /complete almost always means the wallet that signed the
	// nonce doesn't match the address recorded at session creation. The
	// error message must name --address and --skey explicitly so users
	// don't waste a retry on the same flags. Pinned here so a future copy
	// rewrite that drops the hint is a deliberate, test-failing change.
	if !strings.Contains(err.Error(), "--address") || !strings.Contains(err.Error(), "--skey") {
		t.Errorf("err = %q, want hint that names both --address and --skey (the most likely root cause of a 401 at /complete)", err.Error())
	}
}

// -----------------------------------------------------------------------------
// dev refresh (token rotation)
// -----------------------------------------------------------------------------

func TestRunDevRefreshFlow_HappyPath_RotatesAllTokens(t *testing.T) {
	stub := &devGatewayStub{
		refreshRespBody: secureLoginBody(
			"jwt.NEW",
			"refresh.NEW",
			"myalias",
			"dev-user-1",
			"pioneer",
			"2099-01-01T02:00:00Z",
			"2099-02-15T00:00:00Z",
		),
	}
	cfg, _, _ := devTestEnv(t, stub)
	// Pre-populate as if a prior `dev login` had run.
	cfg.DevJWT = "jwt.OLD"
	cfg.DevJWTExpiresAt = "2099-01-01T01:00:00Z"
	cfg.DevRefreshToken = "refresh.OLD"
	cfg.DevRefreshTokenExpiresAt = "2099-02-01T00:00:00Z"
	cfg.DevAlias = "myalias"
	cfg.DevID = "dev-user-1"
	cfg.DevTier = "pioneer"
	cfg.DevKeyHash = "kh"

	if err := runDevRefreshFlow(context.Background(), cfg); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	if !stub.gotRefreshRequest {
		t.Fatal("refresh endpoint was not called")
	}
	if got, want := stub.capturedRefresh["refresh_token"], "refresh.OLD"; got != want {
		t.Errorf("refresh body refresh_token = %v, want %v", got, want)
	}

	if got, want := cfg.DevJWT, "jwt.NEW"; got != want {
		t.Errorf("cfg.DevJWT not rotated; got %q want %q", got, want)
	}
	if got, want := cfg.DevRefreshToken, "refresh.NEW"; got != want {
		t.Errorf("cfg.DevRefreshToken not rotated; got %q want %q", got, want)
	}
	if got, want := cfg.DevJWTExpiresAt, "2099-01-01T02:00:00Z"; got != want {
		t.Errorf("cfg.DevJWTExpiresAt not rotated; got %q want %q", got, want)
	}
	if got, want := cfg.DevRefreshTokenExpiresAt, "2099-02-15T00:00:00Z"; got != want {
		t.Errorf("cfg.DevRefreshTokenExpiresAt not rotated; got %q want %q", got, want)
	}
	// Key hash is set at login (CIP-30 sign), not refresh — must survive a
	// rotation that has nothing to re-sign with.
	if got, want := cfg.DevKeyHash, "kh"; got != want {
		t.Errorf("cfg.DevKeyHash mutated by refresh; got %q want %q (refresh must preserve)", got, want)
	}
}

// TestRunDevRefreshFlow_JSONOutputShape pins the --output json envelope for
// `dev refresh`. Same key-set contract as login except `key_hash` is absent
// (refresh does not re-sign). Token-leak guard runs here too — the rotation
// path is the most credential-dense of all three commands.
func TestRunDevRefreshFlow_JSONOutputShape(t *testing.T) {
	stub := &devGatewayStub{
		refreshRespBody: secureLoginBody(
			"jwt.NEW.SECRET.SHOULD-NOT-LEAK",
			"refresh.NEW.SECRET.SHOULD-NOT-LEAK",
			"myalias",
			"dev-user-1",
			"pioneer",
			"2099-01-01T02:00:00Z",
			"2099-02-15T00:00:00Z",
		),
	}
	cfg, _, _ := devTestEnv(t, stub)
	cfg.DevJWT = "jwt.OLD"
	cfg.DevRefreshToken = "refresh.OLD"
	cfg.DevAlias = "myalias"
	cfg.DevID = "dev-user-1"
	cfg.DevTier = "pioneer"
	cfg.DevKeyHash = "kh"

	captured := captureStdout(t, func() {
		_ = output.SetFormat("json")
		t.Cleanup(func() { _ = output.SetFormat("text") })
		if err := runDevRefreshFlow(context.Background(), cfg); err != nil {
			t.Fatalf("refresh: %v", err)
		}
	})

	if strings.Contains(captured, "SECRET.SHOULD-NOT-LEAK") {
		t.Fatalf("rotated token body leaked to stdout — refresh envelope must NEVER carry the JWT or refresh token. Captured:\n%s", captured)
	}

	var got map[string]interface{}
	if err := json.Unmarshal([]byte(captured), &got); err != nil {
		t.Fatalf("decode dev refresh JSON: %v\nbytes: %s", err, captured)
	}

	for k, want := range map[string]interface{}{
		"alias":                    "myalias",
		"dev_id":                   "dev-user-1",
		"tier":                     "pioneer",
		"jwt_expires_at":           "2099-01-01T02:00:00Z",
		"refresh_token_expires_at": "2099-02-15T00:00:00Z",
	} {
		if got[k] != want {
			t.Errorf("envelope[%q] = %v, want %v", k, got[k], want)
		}
	}
	// key_hash is login-only — refresh does not re-sign so must omit.
	if _, present := got["key_hash"]; present {
		t.Errorf("refresh envelope must not include key_hash (refresh does not re-sign)")
	}
	for _, k := range []string{"jwt", "dev_jwt", "refresh_token", "dev_refresh_token"} {
		if _, present := got[k]; present {
			t.Errorf("envelope must not include %q — token bodies stay on disk only", k)
		}
	}
	if _, present := got["refresh_expires_at"]; present {
		t.Errorf("refresh envelope still uses legacy `refresh_expires_at` — should be `refresh_token_expires_at`")
	}
}

func TestRunDevRefresh_NoRefreshTokenStored_ErrorsWithReloginHint(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	// Empty BaseURL (and empty refresh token) — config.Load skips URL
	// validation when BaseURL is empty, so the cobra wrapper reaches the
	// "no refresh token stored" gate cleanly. Anything we don't care about
	// for this test stays at its zero value.
	if err := config.Save(&config.Config{}); err != nil {
		t.Fatalf("seed empty config: %v", err)
	}

	// Build the cobra command and invoke its RunE — the same path real users
	// hit. SetArgs is unnecessary because the command takes no args.
	cmd := devRefreshCmd
	if err := cmd.RunE(cmd, []string{}); err == nil {
		t.Fatal("expected error when no refresh token stored")
	} else if !strings.Contains(err.Error(), "no refresh token stored") {
		t.Errorf("err = %q, want substring %q", err.Error(), "no refresh token stored")
	}
}

func TestRunDevRefreshFlow_RefreshTokenRejected_ClearsDevSlotAndHintsRelogin(t *testing.T) {
	stub := &devGatewayStub{
		refreshRespStatus: http.StatusUnauthorized,
		refreshRespBody:   []byte(`{"error":"refresh token expired or already rotated"}`),
	}
	cfg, _, _ := devTestEnv(t, stub)
	// Pre-populate the dev slot as if a prior login had succeeded — the
	// 401-clear behavior must wipe ALL of these, not just the refresh token,
	// so `dev status` no longer reports a dead session as valid.
	cfg.DevJWT = "jwt.OLD"
	cfg.DevJWTExpiresAt = "2099-01-01T01:00:00Z"
	cfg.DevRefreshToken = "stale.refresh"
	cfg.DevRefreshTokenExpiresAt = "2099-02-01T00:00:00Z"
	cfg.DevAlias = "myalias"
	cfg.DevID = "dev-user-1"
	cfg.DevTier = "pioneer"
	cfg.DevKeyHash = "kh"
	// User-side fields must NOT be touched by a dev-slot 401.
	cfg.UserJWT = "user.jwt.untouched"
	cfg.UserAlias = "user.alias.untouched"

	err := runDevRefreshFlow(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error on 401 from token/refresh")
	}
	if !strings.Contains(err.Error(), "refresh token rejected") {
		t.Errorf("err = %q, want substring %q (the user-facing prefix)", err.Error(), "refresh token rejected")
	}
	if !strings.Contains(err.Error(), "stored dev credentials cleared") {
		t.Errorf("err = %q, want substring %q (the cleanup-confirmation hint)", err.Error(), "stored dev credentials cleared")
	}
	if !strings.Contains(err.Error(), "andamio dev login") {
		t.Errorf("err = %q, want re-login hint substring %q", err.Error(), "andamio dev login")
	}
	// Underlying typed-auth error must remain reachable for callers that
	// branch on apierr types — `errors.As` unwraps the fmt.Errorf %w wrap.
	var authErr *apierr.AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("expected *apierr.AuthError reachable via errors.As, got %T: %v", err, err)
	}
	if authErr.HTTPStatus != http.StatusUnauthorized {
		t.Errorf("HTTPStatus = %d, want 401", authErr.HTTPStatus)
	}

	// Dev slot must be fully cleared — keeping a known-dead refresh token
	// would silently mislead `dev status` into reporting it as valid.
	if cfg.HasDevAuth() {
		t.Errorf("HasDevAuth() = true after 401; want false (slot must be cleared so dev status reports the truth)")
	}
	if cfg.DevRefreshToken != "" || cfg.DevAlias != "" || cfg.DevTier != "" {
		t.Errorf("dev slot only partially cleared after 401: refresh=%q alias=%q tier=%q", cfg.DevRefreshToken, cfg.DevAlias, cfg.DevTier)
	}
	// User slot must remain untouched — the two slots are independent.
	if cfg.UserJWT != "user.jwt.untouched" || cfg.UserAlias != "user.alias.untouched" {
		t.Errorf("user slot mutated by dev-slot 401: UserJWT=%q UserAlias=%q", cfg.UserJWT, cfg.UserAlias)
	}

	// Disk read-back: prove config.Save persisted the cleared slot.
	loaded, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load after 401-clear: %v", err)
	}
	if loaded.HasDevAuth() {
		t.Errorf("on-disk config still has dev auth after 401; cleanup did not persist")
	}
}

func TestRunDevRefreshFlow_MissingNewJWTErrors(t *testing.T) {
	// Defensive: a successful 200 with empty JWT means the gateway returned
	// junk. Refuse to overwrite the in-flight token with empty.
	stub := &devGatewayStub{
		refreshRespBody: secureLoginBody("" /* no jwt */, "refresh.NEW", "a", "u", "pioneer", "", ""),
	}
	cfg, _, _ := devTestEnv(t, stub)
	cfg.DevJWT = "jwt.OLD"
	cfg.DevRefreshToken = "refresh.OLD"

	err := runDevRefreshFlow(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error when refresh response has empty jwt")
	}
	// Original tokens preserved on error — partial persistence is a footgun.
	if cfg.DevJWT != "jwt.OLD" || cfg.DevRefreshToken != "refresh.OLD" {
		t.Errorf("partial rotation: cfg mutated despite error; jwt=%q refresh=%q", cfg.DevJWT, cfg.DevRefreshToken)
	}
}

// =============================================================================
// dev logout — full-clear + idempotency
// =============================================================================

func TestRunDevLogout_NoAuthStored_Idempotent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := config.Save(&config.Config{}); err != nil {
		t.Fatalf("seed empty config: %v", err)
	}

	cmd := devLogoutCmd
	if err := cmd.RunE(cmd, []string{}); err != nil {
		t.Fatalf("first logout (no auth stored) should not error: %v", err)
	}
	if err := cmd.RunE(cmd, []string{}); err != nil {
		t.Fatalf("second logout (still no auth) should not error: %v", err)
	}
}

// TestRunDevLogout_ClearsRefreshTokenWhenJWTEmpty pins the gate fix from
// PR-A review: if only the refresh token is persisted (env-override path,
// manual config edit, etc.), `dev logout` must still clear it. Pre-fix,
// the gate `!cfg.HasDevAuth()` (JWT-only) returned early and stranded the
// 30-day rotation credential on disk.
func TestRunDevLogout_ClearsRefreshTokenWhenJWTEmpty(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg := &config.Config{
		// JWT empty (e.g., expired and never refreshed) but refresh token
		// still persisted — the strand case the gate fix addresses.
		DevRefreshToken:          "stranded.refresh.token",
		DevRefreshTokenExpiresAt: "2099-02-01T00:00:00Z",
	}
	if err := config.Save(cfg); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	cmd := devLogoutCmd
	if err := cmd.RunE(cmd, []string{}); err != nil {
		t.Fatalf("logout: %v", err)
	}

	loaded, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load after logout: %v", err)
	}
	if loaded.DevRefreshToken != "" {
		t.Errorf("DevRefreshToken = %q after logout; want \"\" (the durable 30-day credential must be cleared even when DevJWT is empty)", loaded.DevRefreshToken)
	}
	if loaded.DevRefreshTokenExpiresAt != "" {
		t.Errorf("DevRefreshTokenExpiresAt = %q after logout; want \"\"", loaded.DevRefreshTokenExpiresAt)
	}
}

// TestRunDevLogout_JSONEnvelope pins the {cleared: bool} envelope shape and
// the distinction between "nothing was stored" (false) vs "wiped real
// credentials" (true). Agents scripting cleanup pipelines branch on this
// to detect whether re-login was needed.
func TestRunDevLogout_JSONEnvelope(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := config.Save(&config.Config{
		DevJWT:          "dev.jwt",
		DevRefreshToken: "dev.refresh",
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	captured := captureStdout(t, func() {
		_ = output.SetFormat("json")
		t.Cleanup(func() { _ = output.SetFormat("text") })
		if err := devLogoutCmd.RunE(devLogoutCmd, []string{}); err != nil {
			t.Fatalf("logout: %v", err)
		}
	})

	var got map[string]interface{}
	if err := json.Unmarshal([]byte(captured), &got); err != nil {
		t.Fatalf("decode: %v\nbytes: %s", err, captured)
	}
	if v, _ := got["cleared"].(bool); !v {
		t.Errorf("cleared = %v on a populated slot; want true", got["cleared"])
	}

	// Second call: no credentials remain — cleared should be false.
	// This exercises the early-return path (line ~370 in dev.go) which is
	// structurally distinct from the populated-slot path. Verify the key is
	// PRESENT-with-value-false, not just falsy from absence — `got["cleared"].(bool)`
	// returns false either way and would silently pass if the early-return
	// regressed to skip JSON emission entirely.
	captured2 := captureStdout(t, func() {
		_ = output.SetFormat("json")
		t.Cleanup(func() { _ = output.SetFormat("text") })
		if err := devLogoutCmd.RunE(devLogoutCmd, []string{}); err != nil {
			t.Fatalf("second logout: %v", err)
		}
	})
	var got2 map[string]interface{}
	if err := json.Unmarshal([]byte(captured2), &got2); err != nil {
		t.Fatalf("decode 2: %v\nbytes: %s", err, captured2)
	}
	v, present := got2["cleared"]
	if !present {
		t.Fatalf("cleared key missing from second-call envelope — early-return path likely did not emit JSON. Captured: %q", captured2)
	}
	if v != false {
		t.Errorf("cleared = %v on second call (no credentials); want false (idempotency contract)", v)
	}
}

func TestRunDevLogout_FullClearPersistsToDisk(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg := &config.Config{
		// User slot — must NOT be cleared by `dev logout`.
		UserJWT:   "user.jwt",
		UserAlias: "user-alias",
		// Dev slot — must be fully cleared.
		DevJWT:                   "dev.jwt",
		DevJWTExpiresAt:          "2099-01-01T01:00:00Z",
		DevRefreshToken:          "dev.refresh",
		DevRefreshTokenExpiresAt: "2099-02-01T00:00:00Z",
		DevAlias:                 "dev-alias",
		DevID:                    "dev-id",
		DevKeyHash:               "kh",
		DevTier:                  "pioneer",
	}
	if err := config.Save(cfg); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	cmd := devLogoutCmd
	if err := cmd.RunE(cmd, []string{}); err != nil {
		t.Fatalf("logout: %v", err)
	}

	loaded, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load after logout: %v", err)
	}
	if loaded.HasDevAuth() {
		t.Errorf("HasDevAuth() = true after logout; want false")
	}
	if loaded.DevRefreshToken != "" || loaded.DevTier != "" || loaded.DevAlias != "" {
		t.Errorf("dev slot not fully cleared on disk: refresh=%q tier=%q alias=%q", loaded.DevRefreshToken, loaded.DevTier, loaded.DevAlias)
	}
	if loaded.UserJWT != "user.jwt" || loaded.UserAlias != "user-alias" {
		t.Errorf("user slot disturbed by dev logout: UserJWT=%q UserAlias=%q", loaded.UserJWT, loaded.UserAlias)
	}
}

// =============================================================================
// dev status — RFC3339 helpers + JSON envelope + text rendering
// =============================================================================

func TestTimeUntil_EmptyStringReturnsAbsent(t *testing.T) {
	expired, remaining, ok := timeUntil("")
	if ok || expired || remaining != 0 {
		t.Errorf("timeUntil(\"\") = (%v, %v, %v); want (false, 0, false)", expired, remaining, ok)
	}
}

func TestTimeUntil_UnparseableReturnsAbsent(t *testing.T) {
	// Genuinely-unparseable timestamps (gateway misbehavior, future format
	// change, manual config edit) must return ok=false so the rendering
	// layer treats the field as absent rather than as expired-or-valid.
	// Note: Go's time.RFC3339 parser is lenient and DOES accept fractional
	// seconds ("…00.123Z") — that's not a parse failure.
	expired, remaining, ok := timeUntil("not-a-timestamp")
	if ok || expired || remaining != 0 {
		t.Errorf("timeUntil(garbage) = (%v, %v, %v); want (false, 0, false)", expired, remaining, ok)
	}
}

func TestTimeUntil_ExpiredReturnsTrue(t *testing.T) {
	// Year-1 timestamp serializes from a zero time.Time; treat as expired.
	expired, _, ok := timeUntil("0001-01-01T00:00:00Z")
	if !ok {
		t.Errorf("timeUntil(year-1) ok = false; want true")
	}
	if !expired {
		t.Errorf("timeUntil(year-1) expired = false; want true (year 1 is always in the past)")
	}
}

func TestTimeUntil_FutureReturnsRemaining(t *testing.T) {
	expired, remaining, ok := timeUntil("2099-01-01T00:00:00Z")
	if !ok {
		t.Fatalf("timeUntil(future) ok = false; want true")
	}
	if expired {
		t.Errorf("timeUntil(future) expired = true; want false")
	}
	if remaining <= 0 {
		t.Errorf("timeUntil(future) remaining = %v; want positive duration", remaining)
	}
}

func TestRunDevStatus_JSON_Unauthenticated(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := config.Save(&config.Config{}); err != nil {
		t.Fatalf("seed empty config: %v", err)
	}

	captured := captureStdout(t, func() {
		_ = output.SetFormat("json")
		t.Cleanup(func() { _ = output.SetFormat("text") })
		if err := devStatusCmd.RunE(devStatusCmd, []string{}); err != nil {
			t.Fatalf("dev status: %v", err)
		}
	})

	var got map[string]interface{}
	if err := json.Unmarshal([]byte(captured), &got); err != nil {
		t.Fatalf("decoding dev status JSON output: %v\nbytes: %s", err, captured)
	}
	if v, _ := got["dev_authenticated"].(bool); v {
		t.Errorf("dev_authenticated = true on empty config; want false")
	}
	if v, _ := got["refresh_token_stored"].(bool); v {
		t.Errorf("refresh_token_stored = true on empty config; want false")
	}
	// Optional fields with omitempty must be absent (not present-as-empty)
	// so consumers can branch on key presence.
	for _, key := range []string{"dev_alias", "dev_id", "dev_tier", "jwt_expires_at", "refresh_token_expires_at"} {
		if _, present := got[key]; present {
			t.Errorf("unauthenticated envelope contains %q; expected omitempty", key)
		}
	}
	// `*_remaining_seconds` is the always-present-with-zero contract — round 2
	// dropped omitempty deliberately (see devStatusResult docstring + CHANGELOG)
	// so sub-second windows surface as 0 instead of being dropped. This pins
	// the contract: a future "cleanup" pass that re-adds omitempty must fail
	// these assertions, not silently drift the JSON envelope.
	for _, key := range []string{"jwt_remaining_seconds", "refresh_token_remaining_seconds"} {
		v, present := got[key]
		if !present {
			t.Errorf("unauthenticated envelope is missing %q; round-2 contract is always-present-with-zero, not omitempty", key)
			continue
		}
		if v != float64(0) {
			t.Errorf("unauthenticated envelope[%q] = %v, want 0", key, v)
		}
	}
}

func TestRunDevStatus_JSON_AuthenticatedSurfacesBothClocksAndTier(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg := &config.Config{
		DevJWT:                   "dev.jwt",
		DevJWTExpiresAt:          "2099-01-01T01:00:00Z",
		DevRefreshToken:          "dev.refresh",
		DevRefreshTokenExpiresAt: "2099-02-01T00:00:00Z",
		DevAlias:                 "myalias",
		DevID:                    "dev-user-1",
		DevTier:                  "pioneer",
		DevKeyHash:               "kh-hex",
	}
	if err := config.Save(cfg); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	captured := captureStdout(t, func() {
		_ = output.SetFormat("json")
		t.Cleanup(func() { _ = output.SetFormat("text") })
		if err := devStatusCmd.RunE(devStatusCmd, []string{}); err != nil {
			t.Fatalf("dev status: %v", err)
		}
	})

	var got map[string]interface{}
	if err := json.Unmarshal([]byte(captured), &got); err != nil {
		t.Fatalf("decoding dev status JSON output: %v\nbytes: %s", err, captured)
	}
	if v, _ := got["dev_authenticated"].(bool); !v {
		t.Errorf("dev_authenticated = false; want true")
	}
	if v, _ := got["refresh_token_stored"].(bool); !v {
		t.Errorf("refresh_token_stored = false; want true")
	}
	if got, want := got["dev_alias"], "myalias"; got != want {
		t.Errorf("dev_alias = %v, want %v", got, want)
	}
	if got, want := got["dev_tier"], "pioneer"; got != want {
		t.Errorf("dev_tier = %v, want %v (must be surfaced for tier-aware scripting)", got, want)
	}
	// Both clocks must surface — agents preemptively refreshing on the
	// 60-min JWT need jwt_remaining_seconds; agents detecting "must
	// re-login" need refresh_token_remaining_seconds.
	if _, ok := got["jwt_remaining_seconds"]; !ok {
		t.Errorf("jwt_remaining_seconds missing from envelope; agents cannot preemptively refresh")
	}
	if _, ok := got["refresh_token_remaining_seconds"]; !ok {
		t.Errorf("refresh_token_remaining_seconds missing from envelope; agents cannot detect must-re-login")
	}
	// jwt_expired and refresh_token_expired are *bool — present only when
	// the timestamp parsed. Both fixtures parse cleanly, so both should
	// be present and false.
	if v, ok := got["jwt_expired"]; !ok || v != false {
		t.Errorf("jwt_expired = %v (ok=%v); want false (token is in the year 2099)", v, ok)
	}
	if v, ok := got["refresh_token_expired"]; !ok || v != false {
		t.Errorf("refresh_token_expired = %v (ok=%v); want false", v, ok)
	}
}

// captureStdout redirects os.Stdout, runs fn, and returns the captured bytes
// as a string. A pipe is used so writes from output.PrintJSON (which calls
// fmt.Println on os.Stdout) land in the buffer rather than the test runner's
// real stdout. Restoring happens via t.Cleanup so a panicking fn doesn't
// strand the redirection across tests.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	old := os.Stdout
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = old })

	done := make(chan []byte, 1)
	go func() {
		var buf [4096]byte
		out := []byte{}
		for {
			n, err := r.Read(buf[:])
			if n > 0 {
				out = append(out, buf[:n]...)
			}
			if err != nil {
				break
			}
		}
		done <- out
	}()

	fn()
	_ = w.Close()
	return string(<-done)
}

// =============================================================================
// Config dev-auth helpers — slot independence + refresh field coverage
// =============================================================================

func TestConfig_HasDevAuth_FalseWhenSlotEmpty(t *testing.T) {
	cfg := &config.Config{}
	if cfg.HasDevAuth() {
		t.Errorf("HasDevAuth() = true on empty config; want false")
	}
}

func TestConfig_HasDevAuth_TrueWhenJWTSet(t *testing.T) {
	cfg := &config.Config{DevJWT: "x.y.z"}
	if !cfg.HasDevAuth() {
		t.Errorf("HasDevAuth() = false despite DevJWT set; want true")
	}
}

func TestConfig_ClearDevAuth_WipesAllSlotFieldsIncludingRefresh(t *testing.T) {
	cfg := &config.Config{
		DevJWT:                   "x.y.z",
		DevJWTExpiresAt:          "2099-01-01T00:00:00Z",
		DevAlias:                 "alias",
		DevID:                    "id",
		DevKeyHash:               "hash",
		DevTier:                  "pioneer",
		DevRefreshToken:          "refresh.token",
		DevRefreshTokenExpiresAt: "2099-02-01T00:00:00Z",
		// User-side fields must NOT be touched — independence between the
		// two slots is the contract.
		UserJWT:   "user.jwt",
		UserAlias: "user-alias",
	}
	cfg.ClearDevAuth()

	if cfg.DevJWT != "" || cfg.DevJWTExpiresAt != "" || cfg.DevAlias != "" ||
		cfg.DevID != "" || cfg.DevKeyHash != "" || cfg.DevTier != "" ||
		cfg.DevRefreshToken != "" || cfg.DevRefreshTokenExpiresAt != "" {
		t.Errorf("ClearDevAuth left dev-side fields populated: %+v", cfg)
	}
	if cfg.UserJWT != "user.jwt" || cfg.UserAlias != "user-alias" {
		t.Errorf("ClearDevAuth touched user-side fields; UserJWT=%q UserAlias=%q (want untouched)", cfg.UserJWT, cfg.UserAlias)
	}
}

// =============================================================================
// dev login dispatcher — branching between browser flow and headless --skey flow
// =============================================================================
//
// Pins the runDevLogin dispatcher's branch matrix. Tests construct fresh
// cobra commands locally rather than mutating the global devLoginCmd, so
// flag state doesn't leak across tests. The dispatcher contract:
//
//   - none of the three flags Changed() → browser branch
//   - all three flags Changed() → headless branch (existing path)
//   - any subset Changed() → partial-flag error
//
// `Changed()` is the discriminator, NOT empty-string equality on
// GetString(). A user running `--skey ""` (e.g., from an unset shell
// variable) sets the flag value to empty but Changed() returns true,
// which must correctly route to the headless branch (where LoadSigningKey
// then fails) rather than mistakenly triggering the browser flow.

// newDevLoginTestCmd constructs a fresh cobra command with the same flag
// shape as the real devLoginCmd, so each test runs against isolated flag
// state. Returns a command whose RunE is the production runDevLogin.
func newDevLoginTestCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "login", RunE: runDevLogin, Args: cobra.NoArgs}
	cmd.Flags().String("skey", "", "Path to .skey file (required for headless mode)")
	cmd.Flags().String("alias", "", "Developer access-token alias (required for headless mode)")
	cmd.Flags().String("address", "", "Bech32 wallet address bound to the access-token alias (required for headless mode)")
	return cmd
}

func TestRunDevLogin_NoFlags_RoutesToBrowser(t *testing.T) {
	// Dispatch test: no flags → browser branch. With Unit 2 landed, the
	// browser branch's first observable behavior is the API-key pre-flight
	// guard (empty APIKey → AuthError mentioning 'auth login --api-key').
	// HOME is set to a tempdir so config.Load returns a fresh empty Config
	// — no API key, no dev creds.
	t.Setenv("HOME", t.TempDir())
	cmd := newDevLoginTestCmd()
	err := cmd.RunE(cmd, []string{})
	if err == nil {
		t.Fatal("expected pre-flight AuthError from browser branch (no API key)")
	}
	if !strings.Contains(err.Error(), "auth login --api-key") {
		t.Errorf("err = %q, want substring %q (verifies dispatch landed in browser branch's API-key pre-flight)", err.Error(), "auth login --api-key")
	}
	var authErr *apierr.AuthError
	if !errors.As(err, &authErr) {
		t.Errorf("expected *apierr.AuthError (exit code 3), got %T: %v", err, err)
	}
}

func TestRunDevLogin_AllThreeFlagsProvided_RoutesToHeadlessAndFailsOnSkey(t *testing.T) {
	// All three flags Changed() must route to runDevHeadlessLogin. We can't
	// run the full headless flow without a real .skey file and a stubbed
	// gateway, so this test asserts the dispatch lands in the headless
	// branch by observing that the error is from LoadSigningKey, not from
	// the partial-flag default branch.
	t.Setenv("HOME", t.TempDir())
	cmd := newDevLoginTestCmd()
	if err := cmd.Flags().Set("skey", "/nonexistent/path.skey"); err != nil {
		t.Fatalf("set skey: %v", err)
	}
	if err := cmd.Flags().Set("alias", "myalias"); err != nil {
		t.Fatalf("set alias: %v", err)
	}
	if err := cmd.Flags().Set("address", "addr_test1..."); err != nil {
		t.Fatalf("set address: %v", err)
	}

	err := cmd.RunE(cmd, []string{})
	if err == nil {
		t.Fatal("expected LoadSigningKey error from headless branch")
	}
	if !strings.Contains(err.Error(), "failed to load signing key") {
		t.Errorf("err = %q, want substring %q (verifies dispatch landed in headless branch)", err.Error(), "failed to load signing key")
	}
	if strings.Contains(err.Error(), "missing:") {
		t.Errorf("err = %q, must NOT contain partial-flag message (would mean we mis-routed)", err.Error())
	}
}

func TestRunDevLogin_PartialFlags_ReturnsErrorNamingBothModes(t *testing.T) {
	// Every subset of {skey, alias, address} that isn't empty or full must
	// route to the partial-flag default branch and name the missing flags.
	cases := []struct {
		name string
		set  []string
	}{
		{"skey only", []string{"skey"}},
		{"alias only", []string{"alias"}},
		{"address only", []string{"address"}},
		{"skey+alias", []string{"skey", "alias"}},
		{"skey+address", []string{"skey", "address"}},
		{"alias+address", []string{"alias", "address"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			cmd := newDevLoginTestCmd()
			for _, flag := range tc.set {
				if err := cmd.Flags().Set(flag, "placeholder"); err != nil {
					t.Fatalf("set %s: %v", flag, err)
				}
			}
			err := cmd.RunE(cmd, []string{})
			if err == nil {
				t.Fatal("expected partial-flag error")
			}
			if !strings.Contains(err.Error(), "missing:") {
				t.Errorf("err = %q, want substring %q", err.Error(), "missing:")
			}
			if !strings.Contains(err.Error(), "browser mode") || !strings.Contains(err.Error(), "headless mode") {
				t.Errorf("err = %q, want both 'browser mode' and 'headless mode' substrings (operator should be able to fix forward either way)", err.Error())
			}
			// Spot-check: each missing flag is named in the error.
			provided := map[string]bool{}
			for _, f := range tc.set {
				provided[f] = true
			}
			for _, flag := range []string{"skey", "alias", "address"} {
				if !provided[flag] && !strings.Contains(err.Error(), "--"+flag) {
					t.Errorf("err = %q, want substring %q (missing flag should be named)", err.Error(), "--"+flag)
				}
			}
		})
	}
}

// =============================================================================
// dev login (browser) — runDevLoginBrowser end-to-end coverage (#100, Unit 2)
// =============================================================================
//
// Pins the dual-credential wire contract, both AuthError pre-flight branches,
// every callback validation guard (state, missing fields, sanitized
// "undefined"), the timeout path, the read-modify-save persistence pattern,
// and the token-leak guard on stdout AND stderr. The user-login browser
// flow is untested in this repo today; do NOT mirror that pattern for the
// dev surface.
//
// `openURL` (package-level var in user.go) is overridden per test to
// simulate what the andamio-app-v2 /auth/dev-cli page would do — parse the
// redirect_uri + state out of the auth URL, then GET the local callback
// with synthetic success/failure params. `devLoginBrowserTimeout` is
// overridden to a short duration so timeout tests don't take 5 minutes.

// devBrowserTestEnv seeds the config with an API key (required to pass the
// browser flow's pre-flight) and any caller-supplied dev-slot or user-slot
// fields. Returns the loaded cfg pointer for direct mutation in tests.
// Sets HOME to a tempdir so config.Load/Save targets isolated state.
func devBrowserTestEnv(t *testing.T, base config.Config) *config.Config {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	cfg := base
	if err := config.Save(&cfg); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return &cfg
}

// overrideOpenURL installs a per-test closure for the browser-opener
// indirection. Restored via t.Cleanup. The closure receives the full auth
// URL the CLI constructed (containing redirect_uri + state in the query
// string) and decides what to do — typically: parse out the redirect_uri,
// issue a GET to it with synthetic callback params.
func overrideOpenURL(t *testing.T, fn func(authURL string) error) {
	t.Helper()
	original := openURL
	openURL = fn
	t.Cleanup(func() { openURL = original })
}

// overrideDevLoginTimeout shortens the browser-flow timeout for tests.
// Always restored via t.Cleanup.
func overrideDevLoginTimeout(t *testing.T, d time.Duration) {
	t.Helper()
	original := devLoginBrowserTimeout
	devLoginBrowserTimeout = d
	t.Cleanup(func() { devLoginBrowserTimeout = original })
}

// successCallback returns an openURL closure that simulates a valid
// /auth/dev-cli callback. The closure parses redirect_uri + state from
// the auth URL and GETs the callback with the supplied query params,
// always echoing back the state token unchanged.
func successCallback(t *testing.T, params url.Values) func(string) error {
	t.Helper()
	return func(authURL string) error {
		u, err := url.Parse(authURL)
		if err != nil {
			return fmt.Errorf("parse authURL: %w", err)
		}
		q := u.Query()
		redirectURI := q.Get("redirect_uri")
		state := q.Get("state")
		callback, err := url.Parse(redirectURI)
		if err != nil {
			return fmt.Errorf("parse redirect_uri: %w", err)
		}
		cbq := callback.Query()
		cbq.Set("state", state) // echo state by default
		for k, vs := range params {
			for _, v := range vs {
				cbq.Set(k, v)
			}
		}
		callback.RawQuery = cbq.Encode()
		resp, err := http.Get(callback.String())
		if err != nil {
			return fmt.Errorf("GET callback: %w", err)
		}
		_ = resp.Body.Close()
		return nil
	}
}

// validCallbackParams returns a baseline set of dev-flow callback fields
// suitable for happy-path tests. Tokens are marked with SECRET.SHOULD-NOT-LEAK
// so the token-leak guards across tests can spot stdout/stderr leakage
// regardless of which call path emits them.
func validCallbackParams() url.Values {
	return url.Values{
		"dev_jwt":                      {"SECRET.SHOULD-NOT-LEAK.jwt"},
		"dev_jwt_expires_at":           {"2099-01-01T01:00:00Z"},
		"dev_refresh_token":            {"SECRET.SHOULD-NOT-LEAK.refresh"},
		"dev_refresh_token_expires_at": {"2099-02-01T00:00:00Z"},
		"alias":                        {"myalias"},
		"dev_id":                       {"dev-uuid-1"},
		"tier":                         {"pioneer"},
		"key_hash":                     {"deadbeef"},
	}
}

func TestRunDevLoginBrowser_HappyPath_PersistsAllSlots(t *testing.T) {
	cfg := devBrowserTestEnv(t, config.Config{
		BaseURL: "https://preprod.api.andamio.io",
		APIKey:  "test-api-key",
	})
	overrideOpenURL(t, successCallback(t, validCallbackParams()))
	overrideDevLoginTimeout(t, 2*time.Second)

	if err := runDevLoginBrowser(context.Background(), cfg); err != nil {
		t.Fatalf("runDevLoginBrowser: %v", err)
	}

	// Verify persistence by re-loading from disk — round-trip proves
	// config.Save actually wrote the dev slot, not just an in-memory update.
	loaded, err := config.Load()
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	wantFields := map[string]struct{ got, want string }{
		"DevJWT":                   {loaded.DevJWT, "SECRET.SHOULD-NOT-LEAK.jwt"},
		"DevJWTExpiresAt":          {loaded.DevJWTExpiresAt, "2099-01-01T01:00:00Z"},
		"DevRefreshToken":          {loaded.DevRefreshToken, "SECRET.SHOULD-NOT-LEAK.refresh"},
		"DevRefreshTokenExpiresAt": {loaded.DevRefreshTokenExpiresAt, "2099-02-01T00:00:00Z"},
		"DevAlias":                 {loaded.DevAlias, "myalias"},
		"DevID":                    {loaded.DevID, "dev-uuid-1"},
		"DevTier":                  {loaded.DevTier, "pioneer"},
		"DevKeyHash":               {loaded.DevKeyHash, "deadbeef"},
	}
	for name, v := range wantFields {
		if v.got != v.want {
			t.Errorf("%s = %q, want %q", name, v.got, v.want)
		}
	}
	// API key preserved by read-modify-save.
	if loaded.APIKey != "test-api-key" {
		t.Errorf("APIKey clobbered: got %q, want %q", loaded.APIKey, "test-api-key")
	}
}

func TestRunDevLoginBrowser_JSONOutputShape_NoTokenLeak(t *testing.T) {
	cfg := devBrowserTestEnv(t, config.Config{
		BaseURL: "https://preprod.api.andamio.io",
		APIKey:  "test-api-key",
	})
	overrideOpenURL(t, successCallback(t, validCallbackParams()))
	overrideDevLoginTimeout(t, 2*time.Second)

	_ = output.SetFormat("json")
	t.Cleanup(func() { _ = output.SetFormat("text") })

	captured := captureStdout(t, func() {
		if err := runDevLoginBrowser(context.Background(), cfg); err != nil {
			t.Fatalf("runDevLoginBrowser: %v", err)
		}
	})

	// Token-leak guard: tokens MUST NOT appear on stdout.
	if strings.Contains(captured, "SECRET.SHOULD-NOT-LEAK") {
		t.Fatalf("token body leaked to stdout: %s", captured)
	}

	// JSON envelope shape — only non-secret metadata.
	var got map[string]interface{}
	if err := json.Unmarshal([]byte(captured), &got); err != nil {
		t.Fatalf("decode envelope: %v\nbytes: %s", err, captured)
	}
	wantFields := map[string]interface{}{
		"alias":                    "myalias",
		"dev_id":                   "dev-uuid-1",
		"tier":                     "pioneer",
		"key_hash":                 "deadbeef",
		"jwt_expires_at":           "2099-01-01T01:00:00Z",
		"refresh_token_expires_at": "2099-02-01T00:00:00Z",
	}
	for name, want := range wantFields {
		if got[name] != want {
			t.Errorf("envelope.%s = %v, want %v", name, got[name], want)
		}
	}
	// Envelope must NOT contain any field whose value is the actual token.
	for k, v := range got {
		if s, ok := v.(string); ok && strings.Contains(s, "SECRET.SHOULD-NOT-LEAK") {
			t.Errorf("envelope.%s contains a token-shaped value (%q) — JSON envelope must never carry token bodies", k, s)
		}
	}
}

func TestRunDevLoginBrowser_BrowserOpenFailure_PrintsURLAndContinues(t *testing.T) {
	// openURL returns an error; runDevLoginBrowser must NOT abort —
	// it should print the URL to stderr and keep listening, then succeed
	// when a callback arrives via an alternate path.
	cfg := devBrowserTestEnv(t, config.Config{
		BaseURL: "https://preprod.api.andamio.io",
		APIKey:  "test-api-key",
	})
	overrideDevLoginTimeout(t, 2*time.Second)

	// First simulate browser-open failure, then deliver a callback in the
	// same closure so the listener gets a result before timeout.
	overrideOpenURL(t, func(authURL string) error {
		// Schedule a callback to fire concurrently with the "browser open
		// failed" return. The runDevLoginBrowser select reads from
		// resultChan after openURL returns.
		go func() {
			if err := successCallback(t, validCallbackParams())(authURL); err != nil {
				t.Errorf("simulated callback after browser-open failure: %v", err)
			}
		}()
		return fmt.Errorf("simulated browser launch failure")
	})

	if err := runDevLoginBrowser(context.Background(), cfg); err != nil {
		t.Fatalf("runDevLoginBrowser should recover from browser-open failure: %v", err)
	}

	// Verify the flow still succeeded — config was written.
	loaded, _ := config.Load()
	if loaded.DevJWT == "" {
		t.Errorf("expected dev JWT persisted after browser-open recovery")
	}
}

func TestRunDevLoginBrowser_KeyHashAbsent_PersistsEmpty(t *testing.T) {
	cfg := devBrowserTestEnv(t, config.Config{
		BaseURL: "https://preprod.api.andamio.io",
		APIKey:  "test-api-key",
	})
	params := validCallbackParams()
	params.Del("key_hash")
	overrideOpenURL(t, successCallback(t, params))
	overrideDevLoginTimeout(t, 2*time.Second)

	if err := runDevLoginBrowser(context.Background(), cfg); err != nil {
		t.Fatalf("runDevLoginBrowser: %v", err)
	}
	loaded, _ := config.Load()
	if loaded.DevKeyHash != "" {
		t.Errorf("DevKeyHash = %q, want empty when callback omits key_hash", loaded.DevKeyHash)
	}
	if loaded.DevJWT == "" {
		t.Errorf("expected other fields to persist even when key_hash is absent")
	}
}

func TestRunDevLoginBrowser_DevJWTUndefined_RejectsAndNoPersistence(t *testing.T) {
	cfg := devBrowserTestEnv(t, config.Config{
		BaseURL: "https://preprod.api.andamio.io",
		APIKey:  "test-api-key",
	})
	params := validCallbackParams()
	params.Set("dev_jwt", "undefined") // sanitizeCallbackValue should drop this
	overrideOpenURL(t, successCallback(t, params))
	overrideDevLoginTimeout(t, 2*time.Second)

	err := runDevLoginBrowser(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error when dev_jwt sanitizes to empty")
	}
	if !strings.Contains(err.Error(), "dev_jwt") {
		t.Errorf("err = %q, want substring naming the missing field", err.Error())
	}
	loaded, _ := config.Load()
	if loaded.DevJWT != "" {
		t.Errorf("DevJWT persisted despite validation failure: %q", loaded.DevJWT)
	}
}

func TestRunDevLoginBrowser_RefreshTokenEmpty_RejectsAndNoPersistence(t *testing.T) {
	// The user-login pattern only validates the JWT, not the refresh
	// token. The dev surface needs BOTH validated — silently persisting
	// an empty refresh token breaks `dev refresh` later with a confusing
	// error. This test is the regression guard.
	cfg := devBrowserTestEnv(t, config.Config{
		BaseURL: "https://preprod.api.andamio.io",
		APIKey:  "test-api-key",
	})
	params := validCallbackParams()
	params.Set("dev_refresh_token", "null") // sanitize → empty
	overrideOpenURL(t, successCallback(t, params))
	overrideDevLoginTimeout(t, 2*time.Second)

	err := runDevLoginBrowser(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error when dev_refresh_token sanitizes to empty")
	}
	if !strings.Contains(err.Error(), "dev_refresh_token") {
		t.Errorf("err = %q, want substring naming dev_refresh_token specifically", err.Error())
	}
	loaded, _ := config.Load()
	if loaded.DevJWT != "" || loaded.DevRefreshToken != "" {
		t.Errorf("partial persistence detected: DevJWT=%q DevRefreshToken=%q", loaded.DevJWT, loaded.DevRefreshToken)
	}
}

func TestRunDevLoginBrowser_AliasEmpty_RejectsAndNoPersistence(t *testing.T) {
	cfg := devBrowserTestEnv(t, config.Config{
		BaseURL: "https://preprod.api.andamio.io",
		APIKey:  "test-api-key",
	})
	params := validCallbackParams()
	params.Del("alias")
	overrideOpenURL(t, successCallback(t, params))
	overrideDevLoginTimeout(t, 2*time.Second)

	err := runDevLoginBrowser(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error when alias is empty")
	}
	if !strings.Contains(err.Error(), "alias") {
		t.Errorf("err = %q, want substring %q", err.Error(), "alias")
	}
	loaded, _ := config.Load()
	if loaded.DevAlias != "" {
		t.Errorf("DevAlias persisted despite validation failure: %q", loaded.DevAlias)
	}
}

func TestRunDevLoginBrowser_StateMismatch_RejectsAndNoPersistence(t *testing.T) {
	cfg := devBrowserTestEnv(t, config.Config{
		BaseURL: "https://preprod.api.andamio.io",
		APIKey:  "test-api-key",
	})
	overrideDevLoginTimeout(t, 2*time.Second)
	// Custom closure that sends a wrong state token in the callback.
	overrideOpenURL(t, func(authURL string) error {
		u, _ := url.Parse(authURL)
		redirectURI := u.Query().Get("redirect_uri")
		callback, _ := url.Parse(redirectURI)
		cbq := callback.Query()
		cbq.Set("state", "wrong-state-not-the-one-the-cli-sent")
		for k, vs := range validCallbackParams() {
			for _, v := range vs {
				cbq.Set(k, v)
			}
		}
		callback.RawQuery = cbq.Encode()
		resp, err := http.Get(callback.String())
		if err != nil {
			return err
		}
		_ = resp.Body.Close()
		return nil
	})

	err := runDevLoginBrowser(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error on state mismatch")
	}
	if !strings.Contains(err.Error(), "state") {
		t.Errorf("err = %q, want substring %q", err.Error(), "state")
	}
	loaded, _ := config.Load()
	if loaded.DevJWT != "" {
		t.Errorf("DevJWT persisted despite state mismatch: %q", loaded.DevJWT)
	}
}

func TestRunDevLoginBrowser_POSTMethod_405_KeepsListening(t *testing.T) {
	// A wrong-method callback must return 405 and keep the listener
	// running. A subsequent valid GET completes the flow.
	cfg := devBrowserTestEnv(t, config.Config{
		BaseURL: "https://preprod.api.andamio.io",
		APIKey:  "test-api-key",
	})
	overrideDevLoginTimeout(t, 2*time.Second)
	overrideOpenURL(t, func(authURL string) error {
		u, _ := url.Parse(authURL)
		redirectURI := u.Query().Get("redirect_uri")
		state := u.Query().Get("state")

		// First: POST → expect 405, listener keeps waiting.
		postResp, err := http.Post(redirectURI, "application/x-www-form-urlencoded", strings.NewReader(""))
		if err != nil {
			return fmt.Errorf("POST: %w", err)
		}
		_ = postResp.Body.Close()
		if postResp.StatusCode != http.StatusMethodNotAllowed {
			return fmt.Errorf("expected 405 on POST, got %d", postResp.StatusCode)
		}

		// Then: valid GET completes the flow.
		callback, _ := url.Parse(redirectURI)
		cbq := callback.Query()
		cbq.Set("state", state)
		for k, vs := range validCallbackParams() {
			for _, v := range vs {
				cbq.Set(k, v)
			}
		}
		callback.RawQuery = cbq.Encode()
		getResp, err := http.Get(callback.String())
		if err != nil {
			return err
		}
		_ = getResp.Body.Close()
		return nil
	})

	if err := runDevLoginBrowser(context.Background(), cfg); err != nil {
		t.Fatalf("runDevLoginBrowser should recover from wrong-method callback: %v", err)
	}
	loaded, _ := config.Load()
	if loaded.DevJWT == "" {
		t.Errorf("expected DevJWT persisted after recovery from 405")
	}
}

func TestRunDevLoginBrowser_GatewayErrorParam_Bubbles(t *testing.T) {
	cfg := devBrowserTestEnv(t, config.Config{
		BaseURL: "https://preprod.api.andamio.io",
		APIKey:  "test-api-key",
	})
	overrideDevLoginTimeout(t, 2*time.Second)
	overrideOpenURL(t, successCallback(t, url.Values{
		"error": {"wallet signature rejected by user"},
	}))

	err := runDevLoginBrowser(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error when callback carries an error param")
	}
	if !strings.Contains(err.Error(), "wallet signature rejected") {
		t.Errorf("err = %q, want gateway error message verbatim", err.Error())
	}
	loaded, _ := config.Load()
	if loaded.DevJWT != "" {
		t.Errorf("DevJWT persisted despite gateway-error callback: %q", loaded.DevJWT)
	}
}

func TestRunDevLoginBrowser_Timeout_NoPersistence(t *testing.T) {
	cfg := devBrowserTestEnv(t, config.Config{
		BaseURL: "https://preprod.api.andamio.io",
		APIKey:  "test-api-key",
	})
	overrideDevLoginTimeout(t, 100*time.Millisecond)
	// No-op openURL: nothing ever GETs the callback, listener times out.
	overrideOpenURL(t, func(authURL string) error { return nil })

	err := runDevLoginBrowser(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("err = %q, want substring %q", err.Error(), "timed out")
	}
	loaded, _ := config.Load()
	if loaded.DevJWT != "" {
		t.Errorf("DevJWT persisted despite timeout: %q", loaded.DevJWT)
	}
}

func TestRunDevLoginBrowser_NoAPIKey_PreFlightAuthError(t *testing.T) {
	// Without an API key, runDevLoginBrowser must short-circuit BEFORE
	// opening the browser. Override openURL to fail loudly if called.
	cfg := devBrowserTestEnv(t, config.Config{
		BaseURL: "https://preprod.api.andamio.io",
		// APIKey deliberately omitted.
	})
	overrideOpenURL(t, func(authURL string) error {
		t.Errorf("openURL must NOT be called when APIKey is empty (pre-flight failed to gate)")
		return nil
	})

	err := runDevLoginBrowser(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected AuthError when APIKey is empty")
	}
	var authErr *apierr.AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("expected *apierr.AuthError, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "auth login --api-key") {
		t.Errorf("err = %q, want hint mentioning 'auth login --api-key'", err.Error())
	}
}

func TestRunDevLoginBrowser_AlreadyAuthed_PreFlightReturnsNil(t *testing.T) {
	// HasDevAuth() pre-flight: existing dev JWT → return nil, no browser.
	cfg := devBrowserTestEnv(t, config.Config{
		BaseURL:  "https://preprod.api.andamio.io",
		APIKey:   "test-api-key",
		DevJWT:   "existing.dev.jwt",
		DevAlias: "existing-alias",
	})
	overrideOpenURL(t, func(authURL string) error {
		t.Errorf("openURL must NOT be called when dev slot is already populated")
		return nil
	})

	if err := runDevLoginBrowser(context.Background(), cfg); err != nil {
		t.Fatalf("expected nil return on already-authed path, got %v", err)
	}
	// Config slot must NOT be touched.
	loaded, _ := config.Load()
	if loaded.DevJWT != "existing.dev.jwt" {
		t.Errorf("existing DevJWT was clobbered: %q", loaded.DevJWT)
	}
	if loaded.DevAlias != "existing-alias" {
		t.Errorf("existing DevAlias was clobbered: %q", loaded.DevAlias)
	}
}

func TestRunDevLoginBrowser_AlreadyAuthed_JSONMode_EmptyStdout(t *testing.T) {
	// In --output json mode, the early-return path emits nothing on
	// stdout (and exits 0). Scripts that need to distinguish
	// freshly-authed from already-authed should call `dev status
	// --output json` after — do NOT synthesize a JSON envelope for this
	// no-op success.
	cfg := devBrowserTestEnv(t, config.Config{
		BaseURL:  "https://preprod.api.andamio.io",
		APIKey:   "test-api-key",
		DevJWT:   "existing.dev.jwt",
		DevAlias: "existing-alias",
	})
	overrideOpenURL(t, func(authURL string) error {
		t.Errorf("openURL must NOT be called")
		return nil
	})

	_ = output.SetFormat("json")
	t.Cleanup(func() { _ = output.SetFormat("text") })

	captured := captureStdout(t, func() {
		if err := runDevLoginBrowser(context.Background(), cfg); err != nil {
			t.Fatalf("expected nil, got %v", err)
		}
	})
	if strings.TrimSpace(captured) != "" {
		t.Errorf("--output json early-return must emit empty stdout; got %q", captured)
	}
}

// TestRunDevLoginBrowser_ConcurrentSlotSafety simulates a concurrent shell
// writing to the user slot between the start of the browser flow and the
// callback. Disk-state form (mutate the file mid-flow) rather than a live
// concurrency simulation — production behavior depends on whether the
// read-modify-save pattern picks up the concurrent write, and a
// deterministic file mutation pins exactly that property.
func TestRunDevLoginBrowser_ConcurrentSlotSafety(t *testing.T) {
	cfg := devBrowserTestEnv(t, config.Config{
		BaseURL: "https://preprod.api.andamio.io",
		APIKey:  "test-api-key",
		// Initial user slot — represents "user JWT existed before browser flow."
		UserJWT:   "initial.user.jwt",
		UserAlias: "initial-user-alias",
	})
	overrideDevLoginTimeout(t, 2*time.Second)

	// Custom closure: mutate config.json directly to simulate a concurrent
	// `user login` updating the UserJWT BEFORE the callback fires. Then
	// trigger the success callback as usual.
	overrideOpenURL(t, func(authURL string) error {
		// Directly overwrite the config file with a "concurrent" user-slot
		// update. The dev flow's read-modify-save must pick this up rather
		// than clobber it with the stale in-memory value.
		concurrent := &config.Config{
			BaseURL:   "https://preprod.api.andamio.io",
			APIKey:    "test-api-key",
			UserJWT:   "CONCURRENTLY-WRITTEN-user.jwt",
			UserAlias: "concurrent-user-alias",
		}
		if err := config.Save(concurrent); err != nil {
			return fmt.Errorf("simulate concurrent save: %w", err)
		}
		return successCallback(t, validCallbackParams())(authURL)
	})

	if err := runDevLoginBrowser(context.Background(), cfg); err != nil {
		t.Fatalf("runDevLoginBrowser: %v", err)
	}

	loaded, _ := config.Load()
	// Dev slot landed.
	if loaded.DevJWT != "SECRET.SHOULD-NOT-LEAK.jwt" {
		t.Errorf("DevJWT = %q, want the callback's value", loaded.DevJWT)
	}
	// User slot reflects the CONCURRENT write, NOT the original in-memory snapshot.
	if loaded.UserJWT != "CONCURRENTLY-WRITTEN-user.jwt" {
		t.Errorf("UserJWT = %q, want the concurrent value (read-modify-save failed: stale data clobbered concurrent write)", loaded.UserJWT)
	}
	if loaded.UserAlias != "concurrent-user-alias" {
		t.Errorf("UserAlias = %q, want the concurrent value", loaded.UserAlias)
	}
}

func TestRunDevLoginBrowser_IntegrationWithAPIKey_DualCredentialsOnWire(t *testing.T) {
	// After browser-flow login succeeds and persists dev creds, a subsequent
	// runAPIKeyJSON call must send BOTH X-API-Key AND Authorization: Bearer
	// <dev_jwt> on the wire. Pins the integration between browser-flow
	// persistence and devKeysClient routing.
	cfg := devBrowserTestEnv(t, config.Config{
		BaseURL: "https://preprod.api.andamio.io",
		APIKey:  "the-api-key-on-wire",
	})
	params := validCallbackParams()
	// Replace SECRET marker with a distinct token we can match on the wire.
	params.Set("dev_jwt", "the-dev-jwt-on-wire")
	overrideOpenURL(t, successCallback(t, params))
	overrideDevLoginTimeout(t, 2*time.Second)

	if err := runDevLoginBrowser(context.Background(), cfg); err != nil {
		t.Fatalf("runDevLoginBrowser: %v", err)
	}

	// Reload to pick up persistence, then stand up an apikey stub and call
	// runAPIKeyJSON. Both headers must ride on the request.
	loaded, _ := config.Load()
	stub := &apikeyGatewayStub{respBody: []byte(`{"ok":true}`)}
	srv := httptest.NewServer(stub.serve())
	t.Cleanup(srv.Close)
	loaded.BaseURL = srv.URL

	if err := runAPIKeyJSON(context.Background(), loaded, "/api/v2/apikey/developer/usage/get"); err != nil {
		t.Fatalf("runAPIKeyJSON: %v", err)
	}
	if got, want := stub.capturedAuthHeader, "Bearer the-dev-jwt-on-wire"; got != want {
		t.Errorf("Authorization = %q, want %q (dev JWT from browser flow must ride here)", got, want)
	}
	if got, want := stub.capturedAPIKeyHeader, "the-api-key-on-wire"; got != want {
		t.Errorf("X-API-Key = %q, want %q (V2AuthMiddleware requires it)", got, want)
	}
}

// TestRunDevLogin_SkeyExplicitEmpty_RoutesToHeadless is the load-bearing
// regression for the `cmd.Flags().Changed()` discriminator. A user running
// `andamio dev login --skey ""` (e.g., from `--skey "$SKEY_PATH"` with
// SKEY_PATH unset) sets the flag value to empty but `Changed("skey")`
// returns true. If the dispatcher used empty-string equality
// (skey == "" && alias == "" && addr == "") it would mistakenly route
// this to the browser branch. With Changed() it correctly routes to
// headless, where LoadSigningKey fails with the existing "missing skey"
// behavior — surfacing the real problem to the operator (empty shell
// variable) rather than silently opening a browser they didn't ask for.
func TestRunDevLogin_SkeyExplicitEmpty_RoutesToHeadless(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cmd := newDevLoginTestCmd()
	// Set --skey to explicit empty value. --alias and --address are NOT
	// set, so this is technically partial — but the important assertion
	// is that --skey "" is treated as "provided" (Changed=true), routing
	// the dispatch via the partial-flag branch (which names --alias and
	// --address as missing), NOT via the browser branch.
	if err := cmd.Flags().Set("skey", ""); err != nil {
		t.Fatalf("set skey to empty: %v", err)
	}

	err := cmd.RunE(cmd, []string{})
	if err == nil {
		t.Fatal("expected partial-flag error (--skey '' counts as provided)")
	}
	if strings.Contains(err.Error(), "not yet implemented") {
		t.Errorf("err = %q, must NOT route to browser branch — --skey '' is provided, not absent. This is the empty-shell-variable regression guard.", err.Error())
	}
	if !strings.Contains(err.Error(), "--alias") || !strings.Contains(err.Error(), "--address") {
		t.Errorf("err = %q, want partial-flag error naming missing --alias and --address", err.Error())
	}
}
