package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// expiredJWT builds a structurally valid (unsigned) JWT whose exp is in the
// past. The client and prerun layers decode exp locally; the signature is
// never checked.
func jwtWithExp(exp time.Time) string {
	enc := func(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) }
	return enc(`{"alg":"HS256","typ":"JWT"}`) + "." + enc(fmt.Sprintf(`{"exp":%d}`, exp.Unix())) + ".sig"
}

// countingStub returns a 200 {"data":[]} server and a counter of requests
// that actually reached it.
func countingStub(t *testing.T) (string, *int64) {
	t.Helper()
	var hits int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&hits, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	t.Cleanup(srv.Close)
	return srv.URL, &hits
}

// runCLIWithJWT is runCLI with a caller-chosen user_jwt value and optional
// extra environment variables.
func runCLIWithJWT(t *testing.T, bin, baseURL, userJWT string, extraEnv []string, args ...string) (stdout, stderr string, code int) {
	t.Helper()

	home := t.TempDir()
	dir := filepath.Join(home, ".andamio")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	cfgMap := map[string]string{
		"base_url": baseURL,
		"api_key":  "test-key",
	}
	if userJWT != "" {
		cfgMap["user_jwt"] = userJWT
	}
	cfg, _ := json.Marshal(cfgMap)
	if err := os.WriteFile(filepath.Join(dir, "config.json"), cfg, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cmd := exec.Command(bin, args...)
	cmd.Env = append(append(os.Environ(), "HOME="+home), extraEnv...)

	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("running %v: %v", args, err)
		}
		code = exitErr.ExitCode()
	}
	return outBuf.String(), errBuf.String(), code
}

// Every JWT-required command — jwtAuthPreRunE parents and the seven
// hand-rolled PreRunEs — must reject a locally-expired session with exit 3 /
// kind auth BEFORE any request leaves the machine (issue #134).
func TestExpiredJWT_FailFastOnJWTRequiredCommands(t *testing.T) {
	bin := buildTestBinary(t)
	expired := jwtWithExp(time.Now().Add(-1 * time.Hour))

	commands := [][]string{
		// jwtAuthPreRunE parents (representatives)
		{"course", "owner", "list"},
		{"teacher", "courses"},
		{"project", "task", "list", "proj-1"},
		// the seven hand-rolled PreRunEs
		{"course", "export", "course-1", "101"},
		{"course", "import", "somedir"},
		{"course", "import-all", "somedir"},
		{"course", "create-module", "--course-id", "course-1"},
		{"tx", "build", "/api/v2/tx/x"},
		{"tx", "register", "--tx-hash", "h", "--tx-type", "t"},
		{"tx", "run", "/api/v2/tx/x", "--skey", "nope.skey", "--tx-type", "t"},
	}

	for _, args := range commands {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			url, hits := countingStub(t)
			argv := append(args, "--output", "json")
			stdout, _, code := runCLIWithJWT(t, bin, url, expired, nil, argv...)

			if code != 3 {
				t.Errorf("exit code = %d, want 3 (auth)", code)
			}
			var parsed map[string]string
			if err := json.Unmarshal([]byte(stdout), &parsed); err != nil {
				t.Fatalf("stdout is not JSON: %v\nraw: %q", err, stdout)
			}
			if parsed["kind"] != "auth" {
				t.Errorf("kind = %q, want auth", parsed["kind"])
			}
			if !strings.Contains(parsed["error"], "session expired at") {
				t.Errorf("error does not name the expiry: %q", parsed["error"])
			}
			if !strings.Contains(parsed["error"], "user login") {
				t.Errorf("error does not name the recovery command: %q", parsed["error"])
			}
			if n := atomic.LoadInt64(hits); n != 0 {
				t.Errorf("%d request(s) reached the gateway; fail-fast must not touch the network", n)
			}
		})
	}
}

// An undecodable token must NOT trip the fail-fast — fail open, gateway
// stays the authority. This is the contract that keeps the "test-jwt"
// fixtures across the suite meaningful.
func TestExpiredJWT_UndecodableTokenFailsOpen(t *testing.T) {
	bin := buildTestBinary(t)
	url, hits := countingStub(t)

	_, _, code := runCLIWithJWT(t, bin, url, "test-jwt", nil, "course", "owner", "list", "--output", "json")

	if code != 0 {
		t.Errorf("exit code = %d, want 0 (request should reach the 200 stub)", code)
	}
	if n := atomic.LoadInt64(hits); n == 0 {
		t.Error("no request reached the gateway; undecodable tokens must be sent as-is")
	}
}

// An expired env-sourced JWT points at ANDAMIO_JWT, not 'user login' — a
// fresh login would be shadowed by the env var on the next Load.
func TestExpiredJWT_EnvSourcedNamesEnvVar(t *testing.T) {
	bin := buildTestBinary(t)
	url, _ := countingStub(t)
	expired := jwtWithExp(time.Now().Add(-1 * time.Hour))

	stdout, _, code := runCLIWithJWT(t, bin, url, "", []string{"ANDAMIO_JWT=" + expired},
		"course", "owner", "list", "--output", "json")

	if code != 3 {
		t.Errorf("exit code = %d, want 3", code)
	}
	var parsed map[string]string
	if err := json.Unmarshal([]byte(stdout), &parsed); err != nil {
		t.Fatalf("stdout is not JSON: %v\nraw: %q", err, stdout)
	}
	if !strings.Contains(parsed["error"], "ANDAMIO_JWT") {
		t.Errorf("error does not name ANDAMIO_JWT: %q", parsed["error"])
	}
	if strings.Contains(parsed["error"], "user login") {
		t.Errorf("env-sourced expiry must not point at 'user login': %q", parsed["error"])
	}
}

// R1 end-to-end: an either-auth command with an expired JWT and a valid API
// key succeeds — the dead token is dropped, the API key rides alone, and the
// warning lands on stderr while stdout stays pure JSON.
func TestExpiredJWT_EitherAuthSucceedsOnAPIKeyAlone(t *testing.T) {
	bin := buildTestBinary(t)
	expired := jwtWithExp(time.Now().Add(-1 * time.Hour))

	var gotAuth atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth.Store(r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	t.Cleanup(srv.Close)

	stdout, stderr, code := runCLIWithJWT(t, bin, srv.URL, expired, nil, "course", "list", "--output", "json")

	if code != 0 {
		t.Errorf("exit code = %d, want 0 — expired JWT must not poison API-key access\nstderr: %q", code, stderr)
	}
	if h, _ := gotAuth.Load().(string); h != "" {
		t.Errorf("expired JWT was sent: Authorization = %q", h)
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &parsed); err != nil {
		t.Fatalf("stdout is not pure JSON: %v\nraw: %q", err, stdout)
	}
	if !strings.Contains(stderr, "stored session expired") {
		t.Errorf("expiry warning missing from stderr: %q", stderr)
	}
}
