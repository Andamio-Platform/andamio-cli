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
	"testing"

	"github.com/spf13/cobra"
)

// =============================================================================
// Developer-portal deprecation (cli#194). Every dev and apikey command prints
// devPortalDeprecationNotice on stderr and otherwise behaves as before, until
// 2.0.0 removes them.
// =============================================================================

const devPortalTestKeyID = "11111111-2222-3333-4444-555555555555"

// devPortalStub answers the routes the deprecated commands call.
func devPortalStub(t *testing.T) string {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/keys", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			_, _ = w.Write([]byte(`{"id":"` + devPortalTestKeyID + `","name":"bot","environment":"preprod","key":"ak_test_RAWVALUE","last4":"alue","created_at":"2026-10-29T00:00:00Z"}`))
			return
		}
		_, _ = w.Write([]byte(`{"keys":[]}`))
	})
	mux.HandleFunc("/api/v2/keys/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/api/v2/apikey/developer/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"alias":"myalias"}}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL
}

// runDevPortalCLI runs the binary with an API key and, when devJWT is set, a
// developer JWT in a fresh HOME.
func runDevPortalCLI(t *testing.T, bin, baseURL, devJWT string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	home := t.TempDir()
	dir := filepath.Join(home, ".andamio")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	cfgMap := map[string]string{"base_url": baseURL, "api_key": "test-key"}
	if devJWT != "" {
		cfgMap["dev_jwt"] = devJWT
	}
	cfg, _ := json.Marshal(cfgMap)
	if err := os.WriteFile(filepath.Join(dir, "config.json"), cfg, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cmd := exec.Command(bin, args...)
	cmd.Env = envWithHome(home)
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

// Each path reaches its handler. dev login and dev refresh fail after the
// notice (no key file, no refresh token), which is enough to prove it prints.
var devPortalDeprecatedPaths = [][]string{
	{"dev", "login", "--skey", "missing.skey", "--alias", "a", "--address", "addr_test1x"},
	{"dev", "logout"},
	{"dev", "refresh"},
	{"dev", "status"},
	{"dev", "keys", "list"},
	{"dev", "keys", "create", "--name", "bot", "--environment", "preprod"},
	{"dev", "keys", "delete", devPortalTestKeyID},
	{"apikey", "usage"},
	{"apikey", "profile"},
}

func TestDevPortalDeprecation_EveryCommandPrintsNoticeOnStderr(t *testing.T) {
	bin := buildTestBinary(t)
	url := devPortalStub(t)

	for _, mode := range []string{"text", "json"} {
		for _, args := range devPortalDeprecatedPaths {
			t.Run(mode+" "+strings.Join(args, " "), func(t *testing.T) {
				stdout, stderr, _ := runDevPortalCLI(t, bin, url, "dev.jwt.value", append(args, "--output", mode)...)
				if n := strings.Count(stderr, devPortalDeprecationNotice); n != 1 {
					t.Errorf("stderr has the notice %d times, want 1\nstderr: %q", n, stderr)
				}
				if strings.Contains(stdout, "api-setup") {
					t.Errorf("notice leaked to stdout: %q", stdout)
				}
			})
		}
	}
}

// The notice must not change what scripts read on stdout.
func TestDevPortalDeprecation_JSONStdoutStillParses(t *testing.T) {
	bin := buildTestBinary(t)
	url := devPortalStub(t)

	for _, args := range [][]string{
		{"dev", "status"},
		{"dev", "keys", "list"},
		{"dev", "keys", "create", "--name", "bot", "--environment", "preprod"},
		{"apikey", "usage"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			stdout, _, code := runDevPortalCLI(t, bin, url, "dev.jwt.value", append(args, "--output", "json")...)
			if code != 0 {
				t.Fatalf("exit code = %d, want 0", code)
			}
			var v interface{}
			if err := json.Unmarshal([]byte(stdout), &v); err != nil {
				t.Errorf("stdout is not JSON: %v\nraw: %q", err, stdout)
			}
		})
	}
}

// 'dev keys create | pbcopy' must still capture only the key.
func TestDevPortalDeprecation_CreateTextStdoutIsOnlyTheKey(t *testing.T) {
	bin := buildTestBinary(t)
	url := devPortalStub(t)

	stdout, stderr, code := runDevPortalCLI(t, bin, url, "dev.jwt.value", "dev", "keys", "create", "--name", "bot", "--environment", "preprod")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0\nstderr: %s", code, stderr)
	}
	if stdout != "ak_test_RAWVALUE\n" {
		t.Errorf("stdout = %q, want the raw key alone", stdout)
	}
}

func TestDevPortalDeprecation_AuthFailureKeepsExitCodeAndKind(t *testing.T) {
	bin := buildTestBinary(t)
	url := devPortalStub(t)

	stdout, stderr, code := runDevPortalCLI(t, bin, url, "", "dev", "keys", "list", "--output", "json")
	if !strings.Contains(stderr, devPortalDeprecationNotice) {
		t.Errorf("stderr missing the notice: %q", stderr)
	}
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
}

func TestDevPortalDeprecation_HelpStaysQuietAndListed(t *testing.T) {
	bin := buildTestBinary(t)
	url := devPortalStub(t)

	_, stderr, _ := runDevPortalCLI(t, bin, url, "", "dev", "keys", "list", "--help")
	if strings.Contains(stderr, devPortalDeprecationNotice) {
		t.Errorf("--help printed the notice: %q", stderr)
	}

	root, _, _ := runDevPortalCLI(t, bin, url, "", "--help")
	for _, name := range []string{"dev", "apikey"} {
		if !strings.Contains(root, "\n  "+name+" ") {
			t.Errorf("root help no longer lists %q", name)
		}
	}
	dev, _, _ := runDevPortalCLI(t, bin, url, "", "dev", "--help")
	for _, name := range []string{"login", "logout", "refresh", "status", "keys"} {
		if !strings.Contains(dev, "\n  "+name+" ") {
			t.Errorf("dev help no longer lists %q", name)
		}
	}
}

// A dev-portal command added later must not skip the notice: the table and
// the leaves under dev and apikey must agree.
func TestDevPortalDeprecation_TableCoversEveryLeaf(t *testing.T) {
	inTable := map[*cobra.Command]bool{}
	for _, cmd := range devPortalDeprecatedCommands {
		inTable[cmd] = true
	}
	leaves := map[*cobra.Command]bool{}
	var walk func(cmd *cobra.Command)
	walk = func(cmd *cobra.Command) {
		if !cmd.HasSubCommands() {
			leaves[cmd] = true
			return
		}
		for _, sub := range cmd.Commands() {
			walk(sub)
		}
	}
	walk(devCmd)
	walk(apikeyCmd)

	for cmd := range leaves {
		if !inTable[cmd] {
			t.Errorf("%q is not in devPortalDeprecatedCommands", cmd.CommandPath())
		}
	}
	for cmd := range inTable {
		if !leaves[cmd] {
			t.Errorf("%q is in devPortalDeprecatedCommands but is not a dev or apikey leaf", cmd.CommandPath())
		}
	}
}
