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

	"github.com/Andamio-Platform/andamio-cli/internal/config"
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
	return runCLIInDir(t, bin, baseURL, userJWT, "", extraEnv, args...)
}

// runCLIInDir is the shared body behind every runCLI* helper: build an isolated
// $HOME with a config pointing at baseURL, run the binary, capture both streams
// and the exit code. workdir sets the child's cwd — needed by commands that read
// or write files relative to it (spec fetch/paths and openapi.json); pass "" to
// inherit the test process's cwd.
func runCLIInDir(t *testing.T, bin, baseURL, userJWT, workdir string, extraEnv []string, args ...string) (stdout, stderr string, code int) {
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
	cmd.Env = append(envWithHome(home), extraEnv...)
	cmd.Dir = workdir

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

// U8: either-auth course reads route by JWT freshness. An expired JWT (with
// API key present) must select the user endpoint and succeed — a
// presence-based route would send it to the teacher endpoint where the
// dropped JWT guarantees a 401.
func TestExpiredJWT_CourseModulesRoutesToUserEndpoint(t *testing.T) {
	bin := buildTestBinary(t)

	cases := []struct {
		name         string
		jwt          string
		wantPathPart string
	}{
		{"expired routes to user endpoint", jwtWithExp(time.Now().Add(-1 * time.Hour)), "/api/v2/course/user/modules/"},
		{"fresh routes to teacher endpoint", jwtWithExp(time.Now().Add(1 * time.Hour)), "/api/v2/course/teacher/course-modules/list"},
		{"undecodable keeps teacher routing (fail open)", "test-jwt", "/api/v2/course/teacher/course-modules/list"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var paths []string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				paths = append(paths, r.URL.Path)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"data":[]}`))
			}))
			t.Cleanup(srv.Close)

			_, _, code := runCLIWithJWT(t, bin, srv.URL, tc.jwt, nil, "course", "modules", "course-1", "--output", "json")

			if code != 0 {
				t.Errorf("exit code = %d, want 0", code)
			}
			found := false
			for _, p := range paths {
				if strings.Contains(p, tc.wantPathPart) {
					found = true
				}
			}
			if !found {
				t.Errorf("no request hit %q; paths seen: %v", tc.wantPathPart, paths)
			}
		})
	}
}

// checkJWTExpiry (tx run pre-check) must judge the token actually in the
// slot, not the stored jwt_expires_at string — an ANDAMIO_JWT override makes
// the stored value describe a different token entirely.
func TestCheckJWTExpiry_PrefersDecodedExpOverStoredString(t *testing.T) {
	fresh := jwtWithExp(time.Now().Add(1 * time.Hour))
	cfg := &config.Config{
		UserJWT:      fresh,
		JWTExpiresAt: "2020-01-01T00:00:00Z", // stale metadata from a previous session
	}
	if err := checkJWTExpiry(cfg, true); err != nil {
		t.Errorf("fresh decodable token rejected because of stale stored expiry: %v", err)
	}

	expired := jwtWithExp(time.Now().Add(-1 * time.Hour))
	cfg = &config.Config{UserJWT: expired, JWTExpiresAt: "2099-01-01T00:00:00Z"}
	if err := checkJWTExpiry(cfg, true); err == nil {
		t.Error("expired decodable token accepted because of fresh stored expiry")
	}
}

func TestCheckJWTExpiry_UndecodableFallsBackToStored(t *testing.T) {
	cfg := &config.Config{UserJWT: "opaque", JWTExpiresAt: "2020-01-01T00:00:00Z"}
	err := checkJWTExpiry(cfg, true)
	if err == nil {
		t.Fatal("expired stored expiry for undecodable token must still hard-fail")
	}
	if !strings.Contains(err.Error(), "session expired at") {
		t.Errorf("unexpected message: %v", err)
	}

	cfg = &config.Config{UserJWT: "opaque"}
	if err := checkJWTExpiry(cfg, true); err != nil {
		t.Errorf("undecodable token without stored expiry must pass: %v", err)
	}
}

// U8 companion coverage: all five either-auth course reads route to user
// endpoints when the JWT is expired.
func TestExpiredJWT_AllCourseReadsRouteToUserEndpoints(t *testing.T) {
	bin := buildTestBinary(t)
	expired := jwtWithExp(time.Now().Add(-1 * time.Hour))

	cases := []struct {
		args         []string
		wantPathPart string
	}{
		{[]string{"course", "slts", "course-1", "101"}, "/api/v2/course/user/slts/"},
		{[]string{"course", "lesson", "course-1", "101", "1"}, "/api/v2/course/user/lesson/"},
		{[]string{"course", "intro", "course-1", "101"}, "/api/v2/course/user/introduction/"},
		{[]string{"course", "assignment", "course-1", "101"}, "/api/v2/course/user/assignment/"},
	}

	for _, tc := range cases {
		t.Run(strings.Join(tc.args[:2], " "), func(t *testing.T) {
			var paths []string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				paths = append(paths, r.URL.Path)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"data":{}}`))
			}))
			t.Cleanup(srv.Close)

			argv := append(tc.args, "--output", "json")
			_, _, code := runCLIWithJWT(t, bin, srv.URL, expired, nil, argv...)

			if code != 0 {
				t.Errorf("exit code = %d, want 0", code)
			}
			for _, p := range paths {
				if strings.Contains(p, "/teacher/") {
					t.Errorf("teacher endpoint %q hit with an expired JWT", p)
				}
			}
			found := false
			for _, p := range paths {
				if strings.Contains(p, tc.wantPathPart) {
					found = true
				}
			}
			if !found {
				t.Errorf("no request hit %q; paths seen: %v", tc.wantPathPart, paths)
			}
		})
	}
}

// A fresh ANDAMIO_JWT must not be judged by a stale on-disk jwt_expires_at
// left over from a previous stored session — neither by enforcement (exit
// codes) nor by the user-status probe.
func TestExpiredJWT_FreshEnvJWTNotJudgedByStaleStoredExpiry(t *testing.T) {
	bin := buildTestBinary(t)
	oldExpired := jwtWithExp(time.Now().Add(-24 * time.Hour))
	freshEnv := jwtWithExp(time.Now().Add(1 * time.Hour))

	home := t.TempDir()
	dir := filepath.Join(home, ".andamio")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// A pre-existing stored session (expired) with its persisted expiry.
	cfg, _ := json.Marshal(map[string]string{
		"base_url":       "http://127.0.0.1:0", // replaced below
		"api_key":        "test-key",
		"user_jwt":       oldExpired,
		"jwt_expires_at": time.Now().Add(-24 * time.Hour).UTC().Format(time.RFC3339),
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	t.Cleanup(srv.Close)
	cfg, _ = json.Marshal(map[string]string{
		"base_url":       srv.URL,
		"api_key":        "test-key",
		"user_jwt":       oldExpired,
		"jwt_expires_at": time.Now().Add(-24 * time.Hour).UTC().Format(time.RFC3339),
	})
	if err := os.WriteFile(filepath.Join(dir, "config.json"), cfg, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	run := func(args ...string) (string, int) {
		cmd := exec.Command(bin, args...)
		cmd.Env = append(envWithHome(home), "ANDAMIO_JWT="+freshEnv)
		var outBuf, errBuf strings.Builder
		cmd.Stdout = &outBuf
		cmd.Stderr = &errBuf
		code := 0
		if err := cmd.Run(); err != nil {
			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) {
				t.Fatalf("running %v: %v", args, err)
			}
			code = exitErr.ExitCode()
		}
		return outBuf.String(), code
	}

	// Enforcement: the fresh env token passes the fail-fast and reaches the
	// gateway.
	if _, code := run("course", "owner", "list", "--output", "json"); code != 0 {
		t.Errorf("course owner list exit = %d, want 0 (fresh env JWT wrongly judged expired)", code)
	}

	// Probe: user status reports the env token's real state, not the stale
	// stored metadata.
	stdout, code := run("user", "status", "--output", "json")
	if code != 0 {
		t.Fatalf("user status exit = %d", code)
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &parsed); err != nil {
		t.Fatalf("stdout not JSON: %v\nraw: %q", err, stdout)
	}
	if v, _ := parsed["session_expired"].(bool); v {
		t.Error("session_expired = true for a fresh ANDAMIO_JWT (stale stored expiry won)")
	}
}
