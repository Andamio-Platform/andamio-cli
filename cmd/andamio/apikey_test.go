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
// apikey usage|profile — andamio-api /v2/apikey/developer/* dual-credential surface
// =============================================================================
//
// Regression guard for the Andrew 401 ("Authorization header with Developer
// JWT required"). The gateway moved /v2/apikey/developer/* behind
// `developerJWTAuth` (main_router.go), the same dual stack as /v2/keys:
// V2AuthMiddleware wants X-API-Key, developerJWTAuth wants Bearer <devJWT>.
// The CLI must therefore route these through devKeysClient, NOT the old
// JWT-stripping helper. These tests pin both halves on the wire.

// apikeyGatewayStub serves the two GET endpoints and records the auth
// headers that actually reached the wire.
type apikeyGatewayStub struct {
	respBody             []byte
	capturedAuthHeader   string
	capturedAPIKeyHeader string
	gotRequest           bool
}

func (s *apikeyGatewayStub) serve() http.Handler {
	mux := http.NewServeMux()
	h := func(w http.ResponseWriter, r *http.Request) {
		s.capturedAuthHeader = r.Header.Get("Authorization")
		s.capturedAPIKeyHeader = r.Header.Get("X-API-Key")
		s.gotRequest = true
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(s.respBody)
	}
	mux.HandleFunc("/api/v2/apikey/developer/usage/get", h)
	mux.HandleFunc("/api/v2/apikey/developer/profile/get", h)
	return mux
}

func apikeyTestEnv(t *testing.T, stub *apikeyGatewayStub) *config.Config {
	t.Helper()
	srv := httptest.NewServer(stub.serve())
	t.Cleanup(srv.Close)
	t.Setenv("HOME", t.TempDir())

	cfg := &config.Config{
		BaseURL:  srv.URL,
		APIKey:   "expected-api-key-on-wire",
		UserJWT:  "tripwire-user-jwt-MUST-NOT-LEAK",
		DevJWT:   "dev.jwt.bearer.value",
		DevAlias: "myalias",
		DevID:    "dev-1",
		DevTier:  "pioneer",
	}
	if err := config.Save(cfg); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	return cfg
}

// TestRunAPIKeyJSON_SendsDualCredential is the direct regression for
// Andrew's report: the request must carry BOTH the dev JWT (Authorization)
// and the api key (X-API-Key). The wallet/user JWT must not leak — the cfg
// clone in devKeysClient overwrites the JWT slot with the dev JWT.
func TestRunAPIKeyJSON_SendsDualCredential(t *testing.T) {
	for _, path := range []string{
		"/api/v2/apikey/developer/usage/get",
		"/api/v2/apikey/developer/profile/get",
	} {
		t.Run(path, func(t *testing.T) {
			stub := &apikeyGatewayStub{respBody: []byte(`{"ok":true}`)}
			cfg := apikeyTestEnv(t, stub)

			if err := runAPIKeyJSON(context.Background(), cfg, path); err != nil {
				t.Fatalf("runAPIKeyJSON: %v", err)
			}
			if !stub.gotRequest {
				t.Fatal("endpoint not called")
			}
			if got, want := stub.capturedAuthHeader, "Bearer dev.jwt.bearer.value"; got != want {
				t.Errorf("Authorization = %q, want %q (dev JWT must ride here, not wallet JWT)", got, want)
			}
			if got, want := stub.capturedAPIKeyHeader, "expected-api-key-on-wire"; got != want {
				t.Errorf("X-API-Key = %q, want %q (V2AuthMiddleware requires it alongside the dev JWT)", got, want)
			}
			// Tripwire: the wallet/user JWT seeded in apikeyTestEnv must be
			// overwritten by the dev JWT in devKeysClient's cfg clone, never
			// appended. Without this negative assertion a header-append
			// regression (emitting both the dev and wallet JWT) would pass
			// the positive checks above unnoticed.
			if strings.Contains(stub.capturedAuthHeader, "tripwire-user-jwt-MUST-NOT-LEAK") {
				t.Errorf("Authorization header leaked the wallet/user JWT: %q", stub.capturedAuthHeader)
			}
		})
	}
}

func TestRunAPIKeyJSON_JSONOutputPrintsBody(t *testing.T) {
	stub := &apikeyGatewayStub{respBody: []byte(`{"tier":"pioneer","calls":42}`)}
	cfg := apikeyTestEnv(t, stub)

	captured := captureStdout(t, func() {
		_ = output.SetFormat("json")
		t.Cleanup(func() { _ = output.SetFormat("text") })
		if err := runAPIKeyJSON(context.Background(), cfg, "/api/v2/apikey/developer/usage/get"); err != nil {
			t.Fatalf("runAPIKeyJSON: %v", err)
		}
	})

	var got map[string]interface{}
	if err := json.Unmarshal([]byte(captured), &got); err != nil {
		t.Fatalf("decode JSON: %v\nbytes: %s", err, captured)
	}
	if got["tier"] != "pioneer" || got["calls"] != float64(42) {
		t.Errorf("body = %v, want tier=pioneer calls=42", got)
	}
}

// TestRunAPIKeyJSON_NoDevAuth_ErrorsWithLoginHint pins the actionable
// remediation. Before the fix, an empty dev slot produced a raw gateway
// 401; now devKeysClient short-circuits with the `dev login` hint.
func TestRunAPIKeyJSON_NoDevAuth_ErrorsWithLoginHint(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg := &config.Config{APIKey: "key-only-no-dev-jwt"}
	if err := config.Save(cfg); err != nil {
		t.Fatalf("seed: %v", err)
	}

	err := runAPIKeyJSON(context.Background(), cfg, "/api/v2/apikey/developer/usage/get")
	if err == nil {
		t.Fatal("expected AuthError when dev slot is empty")
	}
	var authErr *apierr.AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("expected *apierr.AuthError, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "andamio dev login") {
		t.Errorf("err = %q, want substring %q", err.Error(), "andamio dev login")
	}
}

// TestRunAPIKeyJSON_NoAPIKey_ErrorsWithAuthLoginHint pins the symmetric
// remediation for the inverse misconfiguration: dev JWT is present but the
// API key slot is empty. The old getAPIKeyJSON helper carried this hint
// explicitly; the first ce-review pass on the dual-credential refactor
// flagged its removal as a UX regression (an operator who ran `dev login`
// before `auth login` previously got "Run 'andamio auth login --api-key
// <key>'" but landed on a raw gateway 401 after the refactor). The
// pre-check in runAPIKeyJSON restores it; this test pins both that an
// AuthError is returned AND that the hint mentions `auth login --api-key`
// — not the dev-login hint, which would be misleading here.
func TestRunAPIKeyJSON_NoAPIKey_ErrorsWithAuthLoginHint(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg := &config.Config{
		DevJWT:   "dev.jwt.bearer.value",
		DevAlias: "myalias",
		DevID:    "dev-1",
		DevTier:  "pioneer",
	}
	if err := config.Save(cfg); err != nil {
		t.Fatalf("seed: %v", err)
	}

	err := runAPIKeyJSON(context.Background(), cfg, "/api/v2/apikey/developer/usage/get")
	if err == nil {
		t.Fatal("expected AuthError when APIKey slot is empty")
	}
	var authErr *apierr.AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("expected *apierr.AuthError, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "auth login --api-key") {
		t.Errorf("err = %q, want substring %q", err.Error(), "auth login --api-key")
	}
	if strings.Contains(err.Error(), "dev login") {
		t.Errorf("err = %q, must NOT mention dev login (misleading when the missing slot is the API key)", err.Error())
	}
}
