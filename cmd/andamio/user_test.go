package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Andamio-Platform/andamio-cli/internal/config"
	"github.com/Andamio-Platform/andamio-cli/internal/output"
)

// TestSanitizeCallbackValue covers the browser-callback → config → user-status
// pipeline for missing/undefined fields. Locks issue #60's fix: "User ID:
// undefined" never reaches the user, whether they're reading text-mode
// `user status` or parsing `user status --output json`.
func TestSanitizeCallbackValue(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty stays empty", "", ""},
		{"normal value passes through", "usr_abc123", "usr_abc123"},
		{"leading/trailing whitespace trimmed", "  usr_abc  ", "usr_abc"},
		{"JavaScript undefined literal dropped", "undefined", ""},
		{"JavaScript null literal dropped", "null", ""},
		{"whitespace around undefined dropped", "  undefined  ", ""},
		{"case-sensitive: 'Undefined' is a real value", "Undefined", "Undefined"},
		{"case-sensitive: 'UNDEFINED' is a real value", "UNDEFINED", "UNDEFINED"},
		{"whitespace only collapses to empty", "   \t\n  ", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitizeCallbackValue(tc.in)
			if got != tc.want {
				t.Errorf("sanitizeCallbackValue(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestBuildAuthURL locks the API-host → app-host rewrite for the browser CLI
// auth page. The production host has no subdomain prefix (api.andamio.io), so
// a plain ".api." replace silently no-ops and sends the browser to the API
// gateway (which 404s on /auth/cli). Both host shapes must rewrite to .app.
func TestBuildAuthURL(t *testing.T) {
	cases := []struct {
		name    string
		baseURL string
		want    string
	}{
		{"production (no prefix)", "https://api.andamio.io", "https://app.andamio.io/auth/cli?redirect_uri=http%3A%2F%2F127.0.0.1%3A55880%2Fcallback&state=abc"},
		{"preprod (subdomain prefix)", "https://preprod.api.andamio.io", "https://preprod.app.andamio.io/auth/cli?redirect_uri=http%3A%2F%2F127.0.0.1%3A55880%2Fcallback&state=abc"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildAuthURL(tc.baseURL, "/auth/cli", "http://127.0.0.1:55880/callback", "abc")
			if got != tc.want {
				t.Errorf("buildAuthURL(%q) = %q, want %q", tc.baseURL, got, tc.want)
			}
		})
	}
}

func TestAppURLFromBase(t *testing.T) {
	cases := []struct {
		name    string
		baseURL string
		want    string
	}{
		{"production (no prefix)", "https://api.andamio.io", "https://app.andamio.io"},
		{"preprod (subdomain prefix)", "https://preprod.api.andamio.io", "https://preprod.app.andamio.io"},
		{"mainnet (subdomain prefix)", "https://mainnet.api.andamio.io", "https://mainnet.app.andamio.io"},
		{"empty", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := appURLFromBase(tc.baseURL)
			if got != tc.want {
				t.Errorf("appURLFromBase(%q) = %q, want %q", tc.baseURL, got, tc.want)
			}
		})
	}
}

// --- headless user login: expiry persistence + blanked login client (#134) ---

// writeTestSkey writes a cardano-cli-format payment.skey holding a fresh
// ed25519 seed and returns its path.
func writeTestSkey(t *testing.T) string {
	t.Helper()
	seed := make([]byte, 32)
	if _, err := rand.Read(seed); err != nil {
		t.Fatalf("rand: %v", err)
	}
	skey := map[string]string{
		"type":        "PaymentSigningKeyShelley_ed25519",
		"description": "Payment Signing Key",
		"cborHex":     "5820" + hex.EncodeToString(seed),
	}
	data, _ := json.Marshal(skey)
	path := filepath.Join(t.TempDir(), "payment.skey")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write skey: %v", err)
	}
	return path
}

// userLoginStub serves the two-step headless login flow, minting a JWT with
// the given exp, and records the Authorization header seen on each request.
func userLoginStub(t *testing.T, exp time.Time, authHeaders *[]string) *httptest.Server {
	t.Helper()
	jwt := jwtWithExp(exp)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*authHeaders = append(*authHeaders, r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v2/auth/login/session":
			_, _ = w.Write([]byte(`{"id":"sess-1","nonce":"nonce-1","expires_at":"2099-01-01T00:00:00Z"}`))
		case "/api/v2/auth/login/validate":
			_, _ = w.Write([]byte(`{"jwt":"` + jwt + `","user":{"id":"u-1","access_token_alias":"tester"}}`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestRunHeadlessLogin_PersistsDecodedExpiry(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("ANDAMIO_ALLOW_ANY_URL", "1")
	exp := time.Now().Add(90 * time.Minute).Truncate(time.Second)

	var auths []string
	srv := userLoginStub(t, exp, &auths)
	cfg := &config.Config{BaseURL: srv.URL, APIKey: "k"}

	stderr := captureStderr(t, func() {
		if err := runHeadlessLogin(context.Background(), cfg, writeTestSkey(t), "tester", "addr_test1xyz"); err != nil {
			t.Fatalf("runHeadlessLogin: %v", err)
		}
	})

	want := exp.UTC().Format(time.RFC3339)
	if cfg.JWTExpiresAt != want {
		t.Errorf("JWTExpiresAt = %q, want %q (decoded from token exp)", cfg.JWTExpiresAt, want)
	}
	if !strings.Contains(stderr, "Session expires: "+want) {
		t.Errorf("stderr does not report session expiry: %q", stderr)
	}

	// Round-trip through disk: the persisted config carries the expiry.
	reloaded, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if reloaded.JWTExpiresAt != want {
		t.Errorf("persisted jwt_expires_at = %q, want %q", reloaded.JWTExpiresAt, want)
	}
}

// The login client is built from a copy with UserJWT blanked: no stored
// token — expired, fresh, or garbage the CLI cannot decode — may ride on the
// login-session request. This is what makes re-login work without a manual
// logout even for tokens the gateway rejects but the local decoder cannot
// judge (the case the client-level expiry drop alone cannot cover).
func TestRunHeadlessLogin_StoredJWTNeverRidesOnLogin(t *testing.T) {
	for name, stored := range map[string]string{
		"expired decodable": jwtWithExp(time.Now().Add(-1 * time.Hour)),
		"undecodable":       "corrupted-garbage-token",
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("ANDAMIO_ALLOW_ANY_URL", "1")

			var auths []string
			srv := userLoginStub(t, time.Now().Add(1*time.Hour), &auths)
			cfg := &config.Config{BaseURL: srv.URL, APIKey: "k", UserJWT: stored}

			captureStderr(t, func() {
				if err := runHeadlessLogin(context.Background(), cfg, writeTestSkey(t), "tester", "addr_test1xyz"); err != nil {
					t.Fatalf("runHeadlessLogin: %v", err)
				}
			})

			if len(auths) == 0 {
				t.Fatal("no requests reached the stub")
			}
			for i, h := range auths {
				if h != "" {
					t.Errorf("request %d carried Authorization %q; login must never send the stored user JWT", i, h)
				}
			}
		})
	}
}

func TestRunHeadlessLogin_UndecodableGatewayJWTLeavesExpiryEmpty(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("ANDAMIO_ALLOW_ANY_URL", "1")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v2/auth/login/session":
			_, _ = w.Write([]byte(`{"id":"sess-1","nonce":"nonce-1"}`))
		default:
			_, _ = w.Write([]byte(`{"jwt":"opaque-token-format","user":{"id":"u-1"}}`))
		}
	}))
	t.Cleanup(srv.Close)

	cfg := &config.Config{BaseURL: srv.URL, APIKey: "k"}
	captureStderr(t, func() {
		if err := runHeadlessLogin(context.Background(), cfg, writeTestSkey(t), "tester", "addr_test1xyz"); err != nil {
			t.Fatalf("runHeadlessLogin: %v", err)
		}
	})

	if cfg.JWTExpiresAt != "" {
		t.Errorf("JWTExpiresAt = %q, want empty for undecodable token", cfg.JWTExpiresAt)
	}
	if cfg.UserJWT != "opaque-token-format" {
		t.Errorf("UserJWT = %q; undecodable tokens must still be stored", cfg.UserJWT)
	}
}

// --- user status: decoded-exp fallback + skew-aligned probe (#134) ---

// statusJSON runs the JSON branch of user status against a scratch HOME
// seeded with cfgMap and returns the parsed envelope.
func statusJSON(t *testing.T, cfgMap map[string]string) map[string]interface{} {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	dir := filepath.Join(os.Getenv("HOME"), ".andamio")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if cfgMap["base_url"] == "" {
		cfgMap["base_url"] = "https://preprod.api.andamio.io"
	}
	data, _ := json.Marshal(cfgMap)
	if err := os.WriteFile(filepath.Join(dir, "config.json"), data, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if err := output.SetFormat("json"); err != nil {
		t.Fatalf("SetFormat: %v", err)
	}
	t.Cleanup(func() { _ = output.SetFormat("text") })

	raw := captureStdout(t, func() {
		if err := runUserStatus(nil, nil); err != nil {
			t.Fatalf("runUserStatus: %v", err)
		}
	})
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		t.Fatalf("stdout is not JSON: %v\nraw: %q", err, raw)
	}
	return parsed
}

func TestUserStatus_DecodedExpFallback_Expired(t *testing.T) {
	// Pre-fix shape: headless login stored a JWT but no jwt_expires_at.
	expired := jwtWithExp(time.Now().Add(-1 * time.Hour))
	parsed := statusJSON(t, map[string]string{"api_key": "k", "user_jwt": expired})

	if v, _ := parsed["session_expired"].(bool); !v {
		t.Errorf("session_expired = %v, want true (decoded from token exp)", parsed["session_expired"])
	}
	if parsed["session_expires_at"] == nil {
		t.Error("session_expires_at absent; decoded fallback should surface it")
	}
}

func TestUserStatus_DecodedExpFallback_Fresh(t *testing.T) {
	fresh := jwtWithExp(time.Now().Add(2 * time.Hour))
	parsed := statusJSON(t, map[string]string{"api_key": "k", "user_jwt": fresh})

	if v, _ := parsed["session_expired"].(bool); v {
		t.Error("session_expired = true for fresh token")
	}
	if secs, _ := parsed["session_remaining_seconds"].(float64); secs <= 0 {
		t.Errorf("session_remaining_seconds = %v, want > 0", parsed["session_remaining_seconds"])
	}
}

func TestUserStatus_UndecodableTokenKeepsLegacyShape(t *testing.T) {
	parsed := statusJSON(t, map[string]string{"api_key": "k", "user_jwt": "test-jwt"})

	if _, present := parsed["session_expires_at"]; present {
		t.Error("session_expires_at present for undecodable token; legacy no-expiry shape must be preserved")
	}
	if _, present := parsed["session_expired"]; present {
		t.Error("session_expired present for undecodable token")
	}
	if v, _ := parsed["user_authenticated"].(bool); !v {
		t.Error("user_authenticated must remain true (presence-based)")
	}
}

func TestUserStatus_StoredExpiryTakesPrecedenceOverDecodedExp(t *testing.T) {
	// Stored says 2099; token says expired an hour ago. Stored wins.
	expiredToken := jwtWithExp(time.Now().Add(-1 * time.Hour))
	parsed := statusJSON(t, map[string]string{
		"api_key":        "k",
		"user_jwt":       expiredToken,
		"jwt_expires_at": "2099-01-01T00:00:00Z",
	})

	if got := parsed["session_expires_at"]; got != "2099-01-01T00:00:00Z" {
		t.Errorf("session_expires_at = %v, want stored value", got)
	}
	if v, _ := parsed["session_expired"].(bool); v {
		t.Error("session_expired = true; stored expiry must take precedence")
	}
}

// The probe uses the same skew as enforcement: a token expiring inside the
// 30s window reads as expired here, matching the exit-3 the next command
// would produce.
func TestUserStatus_ProbeAgreesWithEnforcementSkew(t *testing.T) {
	insideSkew := jwtWithExp(time.Now().Add(config.ExpirySkew - 2*time.Second))
	parsed := statusJSON(t, map[string]string{"api_key": "k", "user_jwt": insideSkew})

	if v, _ := parsed["session_expired"].(bool); !v {
		t.Error("session_expired = false inside the skew window; probe and enforcement disagree")
	}
}

func TestUserStatus_TextHintDropsLogoutStep(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := filepath.Join(os.Getenv("HOME"), ".andamio")
	_ = os.MkdirAll(dir, 0o700)
	expired := jwtWithExp(time.Now().Add(-1 * time.Hour))
	data, _ := json.Marshal(map[string]string{
		"base_url": "https://preprod.api.andamio.io", "api_key": "k", "user_jwt": expired,
	})
	_ = os.WriteFile(filepath.Join(dir, "config.json"), data, 0o600)

	stdout := captureStdout(t, func() {
		if err := runUserStatus(nil, nil); err != nil {
			t.Fatalf("runUserStatus: %v", err)
		}
	})

	if !strings.Contains(stdout, "EXPIRED") {
		t.Errorf("text mode does not report EXPIRED: %q", stdout)
	}
	if strings.Contains(stdout, "logout &&") {
		t.Errorf("stale two-step hint survives: %q", stdout)
	}
	if !strings.Contains(stdout, "Run 'andamio user login' to re-authenticate.") {
		t.Errorf("missing single-step recovery hint: %q", stdout)
	}
}
