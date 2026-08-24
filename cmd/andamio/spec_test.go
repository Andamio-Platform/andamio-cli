package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// runSpecCLI runs the binary with an isolated cwd, because spec fetch writes
// openapi.json there and spec paths reads it from there. Without the isolation
// these tests would write into the package directory and read each other's
// leftovers. Everything else is the shared runCLIInDir body.
func runSpecCLI(t *testing.T, bin, baseURL, workdir string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	return runCLIInDir(t, bin, baseURL, "", workdir, nil, args...)
}

// backdate returns a wall-clock offset, not a calendar offset. The code under
// test computes age as time.Since(mtime).Hours()/24, so a fixture built with
// AddDate(0, 0, -N) disagrees with it by an hour whenever the window spans a
// DST transition — 90 calendar days across a spring-forward is 2159h, which
// floors to 89, not 90. That made this suite fail roughly a quarter of the
// calendar year in any DST timezone, and pass on the day it was written.
func backdate(days int) time.Time {
	return time.Now().Add(-time.Duration(days) * 24 * time.Hour)
}

// writeLocalSpec drops an openapi.json into dir with the given mtime.
func writeLocalSpec(t *testing.T, dir, body string, mtime time.Time) string {
	t.Helper()
	p := filepath.Join(dir, "openapi.json")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatalf("write local spec: %v", err)
	}
	if err := os.Chtimes(p, mtime, mtime); err != nil {
		t.Fatalf("backdate local spec: %v", err)
	}
	return p
}

const specDocBody = `{"swagger":"2.0","info":{"title":"Andamio API Gateway","version":"2.5.0"},"paths":{"/v2/tx/types":{"get":{"summary":"List TX types"}}}}`

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
			_, _ = w.Write([]byte(specDocBody))
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

// spec paths' network fallback is the second use-site of specDocPath and the
// one no test covered — a regression isolated to this branch would have shipped.
// It fires only when there is no local openapi.json.
func TestSpecPaths_NetworkFallbackUsesLiveGatewayRoute(t *testing.T) {
	bin := buildTestBinary(t)
	url, requested := specStub(t)
	work := t.TempDir() // deliberately empty — no local openapi.json

	stdout, stderr, code := runSpecCLI(t, bin, url, work, "spec", "paths", "--output", "json")
	if code != 0 {
		t.Fatalf("exit = %d, want 0\nstdout: %q\nstderr: %q", code, stdout, stderr)
	}
	for _, p := range requested() {
		if strings.Contains(p, "/api/v1/docs/doc.json") {
			t.Errorf("fallback requested the sunset route %q", p)
		}
	}
	if got := requested(); len(got) == 0 || got[0] != specDocPath {
		t.Errorf("fallback requested %v, want %q", got, specDocPath)
	}
	if strings.Contains(stderr, "using local") {
		t.Errorf("claimed a local spec when none existed: %q", stderr)
	}
	var entries []specPathEntry
	if err := json.Unmarshal([]byte(stdout), &entries); err != nil {
		t.Fatalf("stdout is not a JSON array: %v\nraw: %q", err, stdout)
	}
	if len(entries) == 0 {
		t.Error("fallback returned no paths")
	}
}

// A non-200 on the fallback used to parse cleanly, fail the spec["paths"]
// assertion, and surface as a bare "no paths found in spec" — naming neither
// the status nor the URL. Anyone on a pre-2.5 gateway lands here.
func TestSpecPaths_NetworkFallbackReportsHTTPStatus(t *testing.T) {
	bin := buildTestBinary(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(404)
		_, _ = w.Write([]byte(`{"status_code":404,"message":"Not Found"}`))
	}))
	t.Cleanup(srv.Close)

	stdout, stderr, code := runSpecCLI(t, bin, srv.URL, t.TempDir(), "spec", "paths")
	if code == 0 {
		t.Fatalf("exit = 0 on a 404 fallback, want non-zero\nstdout: %q", stdout)
	}
	combined := stdout + stderr
	if !strings.Contains(combined, "404") {
		t.Errorf("error does not name the HTTP status: %q", combined)
	}
	if strings.Contains(combined, "no paths found in spec") {
		t.Errorf("still reporting the downstream symptom instead of the cause: %q", combined)
	}
}

// A stale local openapi.json lists routes the gateway has since removed. #137
// records that this silently sent the #133 dry run down a dead path, so the
// fallback has to announce itself — on stderr, leaving stdout parseable.
func TestSpecPaths_WarnsWhenServingStaleLocalSpec(t *testing.T) {
	bin := buildTestBinary(t)
	url, _ := specStub(t)
	work := t.TempDir()
	writeLocalSpec(t, work,
		`{"swagger":"2.0","paths":{"/v2/course/student/commitment/create":{"post":{"summary":"Sunset route"}}}}`,
		backdate(90))

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

// The staleness notice must fire under --output json too. Suppressing it there
// would leave the scripting surface — the consumer least able to notice — with
// no signal at all, since a bare []specPathEntry looks identical whether it came
// from a fresh fetch or a year-old file. stdout still has to stay clean.
func TestSpecPaths_StaleNoticeAlsoFiresInJSONMode(t *testing.T) {
	bin := buildTestBinary(t)
	url, _ := specStub(t)
	work := t.TempDir()
	writeLocalSpec(t, work, `{"paths":{"/v2/tx/types":{"get":{"summary":"x"}}}}`, backdate(30))

	stdout, stderr, code := runSpecCLI(t, bin, url, work, "spec", "paths", "--output", "json")
	if code != 0 {
		t.Fatalf("exit = %d, want 0\nstderr: %q", code, stderr)
	}
	if !strings.Contains(stderr, "using local openapi.json") {
		t.Errorf("stale notice was suppressed in JSON mode: %q", stderr)
	}
	if !strings.Contains(stderr, "30 days old") {
		t.Errorf("notice does not carry the file's age: %q", stderr)
	}
	if strings.Contains(stdout, "using local") {
		t.Errorf("notice leaked into the JSON payload: %q", stdout)
	}
	var entries []specPathEntry
	if err := json.Unmarshal([]byte(stdout), &entries); err != nil {
		t.Fatalf("stdout is not a clean JSON array: %v\nraw: %q", err, stdout)
	}
}

// A future mtime (clock skew, NFS, an mtime-restoring checkout) must not render
// a negative day count.
func TestSpecPaths_FutureMtimeDoesNotReportNegativeAge(t *testing.T) {
	bin := buildTestBinary(t)
	url, _ := specStub(t)
	work := t.TempDir()
	writeLocalSpec(t, work, `{"paths":{"/v2/tx/types":{"get":{"summary":"x"}}}}`, backdate(-3))

	_, stderr, code := runSpecCLI(t, bin, url, work, "spec", "paths")
	if code != 0 {
		t.Fatalf("exit = %d, want 0\nstderr: %q", code, stderr)
	}
	if strings.Contains(stderr, "-") && strings.Contains(stderr, "days old") {
		if strings.Contains(stderr, "(-") {
			t.Errorf("negative day count rendered: %q", stderr)
		}
	}
	if !strings.Contains(stderr, "0 days old") {
		t.Errorf("future mtime should clamp to 0 days: %q", stderr)
	}
}
