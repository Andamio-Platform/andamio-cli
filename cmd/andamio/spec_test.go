package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// runSpecCLI is runCLIWithJWT plus a working directory, because spec fetch
// writes openapi.json to the cwd and spec paths reads it from there. Without an
// isolated cwd these tests would write into the package directory and read each
// other's leftovers.
func runSpecCLI(t *testing.T, bin, baseURL, workdir string, args ...string) (stdout, stderr string, code int) {
	t.Helper()

	home := t.TempDir()
	dir := filepath.Join(home, ".andamio")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	cfg, _ := json.Marshal(map[string]string{"base_url": baseURL, "api_key": "test-key"})
	if err := os.WriteFile(filepath.Join(dir, "config.json"), cfg, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cmd := exec.Command(bin, args...)
	cmd.Env = envWithHome(home)
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

// specStub serves the OpenAPI document at the live gateway route and records
// every path requested, so a regression back to the sunset route is visible as
// a wrong path rather than only as a downstream failure.
func specStub(t *testing.T) (url string, requested func() []string) {
	t.Helper()
	var (
		mu   sync.Mutex
		seen []string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.URL.Path)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == specDocPath {
			_, _ = w.Write([]byte(`{"swagger":"2.0","info":{"title":"Andamio API Gateway","version":"2.5.0"},"paths":{"/v2/tx/types":{"get":{"summary":"List TX types"}}}}`))
			return
		}
		w.WriteHeader(404)
		_, _ = w.Write([]byte(`{"status_code":404,"message":"Not Found"}`))
	}))
	t.Cleanup(srv.Close)
	return srv.URL, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), seen...)
	}
}

// andamio-api#652 removed /api/v1/docs/doc.json on 2026-07-28 and the CLI kept
// requesting it, so spec fetch 404'd against every current gateway (#137). Both
// spec fetch and spec paths' network fallback hardcoded the dead route
// separately — this pins the live route for both.
func TestSpecFetch_UsesLiveGatewayRoute(t *testing.T) {
	bin := buildTestBinary(t)
	url, requested := specStub(t)
	work := t.TempDir()

	stdout, stderr, code := runSpecCLI(t, bin, url, work, "spec", "fetch", "--output", "json")
	if code != 0 {
		t.Fatalf("exit = %d, want 0\nstdout: %q\nstderr: %q", code, stdout, stderr)
	}

	for _, p := range requested() {
		if strings.Contains(p, "/api/v1/docs/doc.json") {
			t.Errorf("requested the sunset route %q — andamio-api#652 removed it", p)
		}
	}
	if got := requested(); len(got) == 0 || got[0] != specDocPath {
		t.Errorf("requested %v, want first request to %q", got, specDocPath)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("stdout is not JSON: %v\nraw: %q", err, stdout)
	}
	if payload["api_version"] != "2.5.0" {
		t.Errorf("api_version = %v, want 2.5.0", payload["api_version"])
	}
	if _, err := os.Stat(filepath.Join(work, "openapi.json")); err != nil {
		t.Errorf("openapi.json was not written: %v", err)
	}
}

// A stale local openapi.json lists routes the gateway has since removed. #137
// records that this silently sent the #133 dry run down a dead path, so the
// fallback has to announce itself — on stderr, leaving stdout parseable.
func TestSpecPaths_WarnsWhenServingStaleLocalSpec(t *testing.T) {
	bin := buildTestBinary(t)
	url, _ := specStub(t)
	work := t.TempDir()

	local := filepath.Join(work, "openapi.json")
	body := `{"swagger":"2.0","paths":{"/v2/course/student/commitment/create":{"post":{"summary":"Sunset route"}}}}`
	if err := os.WriteFile(local, []byte(body), 0o644); err != nil {
		t.Fatalf("write local spec: %v", err)
	}
	stale := time.Now().AddDate(0, 0, -90)
	if err := os.Chtimes(local, stale, stale); err != nil {
		t.Fatalf("backdate local spec: %v", err)
	}

	stdout, stderr, code := runSpecCLI(t, bin, url, work, "spec", "paths")
	if code != 0 {
		t.Fatalf("exit = %d, want 0\nstderr: %q", code, stderr)
	}
	if !strings.Contains(stderr, "using local openapi.json") {
		t.Errorf("no stale-spec notice on stderr: %q", stderr)
	}
	if !strings.Contains(stderr, "90 days old") {
		t.Errorf("notice does not carry the file's age: %q", stderr)
	}
	if strings.Contains(stdout, "using local") {
		t.Errorf("notice leaked to stdout, which must stay parseable: %q", stdout)
	}
	if !strings.Contains(stdout, "/v2/course/student/commitment/create") {
		t.Errorf("stdout did not carry the local spec's paths: %q", stdout)
	}
}

// The notice is a human aid, not part of the scripting surface: --output json
// must stay clean so `spec paths --output json | jq` is unaffected.
func TestSpecPaths_StaleNoticeSuppressedInJSONMode(t *testing.T) {
	bin := buildTestBinary(t)
	url, _ := specStub(t)
	work := t.TempDir()

	local := filepath.Join(work, "openapi.json")
	if err := os.WriteFile(local, []byte(`{"paths":{"/v2/tx/types":{"get":{"summary":"x"}}}}`), 0o644); err != nil {
		t.Fatalf("write local spec: %v", err)
	}
	stale := time.Now().AddDate(0, 0, -30)
	_ = os.Chtimes(local, stale, stale)

	stdout, stderr, code := runSpecCLI(t, bin, url, work, "spec", "paths", "--output", "json")
	if code != 0 {
		t.Fatalf("exit = %d, want 0\nstderr: %q", code, stderr)
	}
	if strings.Contains(stderr, "using local") {
		t.Errorf("stale notice fired in JSON mode: %q", stderr)
	}
	var entries []specPathEntry
	if err := json.Unmarshal([]byte(stdout), &entries); err != nil {
		t.Fatalf("stdout is not a JSON array: %v\nraw: %q", err, stdout)
	}
}
