package main

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Andamio-Platform/andamio-cli/internal/apierr"
	"github.com/Andamio-Platform/andamio-cli/internal/config"
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

func TestRunDevRefreshFlow_RefreshTokenRejected_BubblesAuthErrWithReloginHint(t *testing.T) {
	stub := &devGatewayStub{
		refreshRespStatus: http.StatusUnauthorized,
		refreshRespBody:   []byte(`{"error":"refresh token expired or already rotated"}`),
	}
	cfg, _, _ := devTestEnv(t, stub)
	cfg.DevRefreshToken = "stale.refresh"

	err := runDevRefreshFlow(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error on 401 from token/refresh")
	}
	if !strings.Contains(err.Error(), "refresh token rejected") {
		t.Errorf("err = %q, want substring %q (the user-facing prefix)", err.Error(), "refresh token rejected")
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
