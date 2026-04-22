package main

import (
	"encoding/json"
	"errors"
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

// TestShortCommit guards against the commit[:7] panic on compile-time defaults.
func TestShortCommit(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"", ""},
		{"abc", "abc"},             // shorter than 7 — returned unchanged
		{"none", "none"},           // default value, 4 chars
		{"abcdef", "abcdef"},       // 6 chars — still no truncation
		{"abcdefg", "abcdefg"},     // exactly 7 — boundary, no truncation needed
		{"abcdefgh", "abcdefg"},    // 8 chars — truncated to 7
		{"abcdef1234567890", "abcdef1"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := shortCommit(tt.input); got != tt.want {
				t.Errorf("shortCommit(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestBuildVersionOutput_TextMode locks the human-readable format. Mutates the
// package-level vars so the test is self-contained; restores them on cleanup.
func TestBuildVersionOutput_TextMode(t *testing.T) {
	restore := setVersionVars(t, "0.12.0", "abc1234def5678", "2026-04-22T12:00:00Z", "text")
	defer restore()

	got := buildVersionOutput()
	wantPrefix := "andamio 0.12.0 (commit: abc1234, built: 2026-04-22T12:00:00Z)\n"
	if got != wantPrefix {
		t.Errorf("text output = %q, want %q", got, wantPrefix)
	}
}

// TestBuildVersionOutput_JSONMode locks the {version, commit, built} envelope.
func TestBuildVersionOutput_JSONMode(t *testing.T) {
	restore := setVersionVars(t, "0.12.0", "abc1234def5678", "2026-04-22T12:00:00Z", "json")
	defer restore()

	got := buildVersionOutput()
	if !strings.HasSuffix(got, "\n") {
		t.Errorf("JSON output missing trailing newline: %q", got)
	}

	var parsed map[string]string
	if err := json.Unmarshal([]byte(strings.TrimSpace(got)), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v; raw = %q", err, got)
	}
	wantKeys := []string{"version", "commit", "built"}
	for _, k := range wantKeys {
		if _, ok := parsed[k]; !ok {
			t.Errorf("missing key %q: %v", k, parsed)
		}
	}
	if len(parsed) != len(wantKeys) {
		t.Errorf("envelope has %d keys, want exactly %d: %v", len(parsed), len(wantKeys), parsed)
	}
	if parsed["version"] != "0.12.0" {
		t.Errorf("version = %q, want 0.12.0", parsed["version"])
	}
	if parsed["commit"] != "abc1234" {
		t.Errorf("commit = %q, want truncated to 7 chars", parsed["commit"])
	}
	if parsed["built"] != "2026-04-22T12:00:00Z" {
		t.Errorf("built = %q, want 2026-04-22T12:00:00Z", parsed["built"])
	}
}

// TestBuildVersionOutput_JSONMode_AtCompileTimeDefaults exercises the edge case where
// ldflags haven't been injected (dev builds). The shortCommit guard must prevent a
// panic on commit = "none" (4 chars).
func TestBuildVersionOutput_JSONMode_AtCompileTimeDefaults(t *testing.T) {
	restore := setVersionVars(t, "dev", "none", "unknown", "json")
	defer restore()

	got := buildVersionOutput()
	var parsed map[string]string
	if err := json.Unmarshal([]byte(strings.TrimSpace(got)), &parsed); err != nil {
		t.Fatalf("invalid JSON at defaults: %v; raw = %q", err, got)
	}
	if parsed["commit"] != "none" {
		t.Errorf("commit at default = %q, want \"none\" (unchanged by shortCommit)", parsed["commit"])
	}
}

// TestBuildVersionOutput_EmptyOutputFormat locks the "before flag parsing" / "never set"
// case to text mode. Ensures an un-initialized outputFormat does not accidentally route to
// JSON.
func TestBuildVersionOutput_EmptyOutputFormat(t *testing.T) {
	restore := setVersionVars(t, "0.12.0", "abcdef1", "2026-04-22", "")
	defer restore()

	got := buildVersionOutput()
	if !strings.HasPrefix(got, "andamio ") {
		t.Errorf("empty outputFormat did not default to text: %q", got)
	}
}

// TestVersionFlag_TextMode_Integration runs the built binary and asserts the plain-text
// --version output is unchanged from pre-refactor. Proves Cobra's template wiring works
// end-to-end.
func TestVersionFlag_TextMode_Integration(t *testing.T) {
	bin := buildTestBinary(t)
	out, err := exec.Command(bin, "--version").Output()
	if err != nil {
		t.Fatalf("binary failed: %v", err)
	}
	// Shape: "andamio <version> (commit: <c>, built: <d>)\n"
	want := regexp.MustCompile(`^andamio \S+ \(commit: \S+, built: \S+\)\n$`)
	if !want.Match(out) {
		t.Errorf("--version (text) did not match expected skeleton: %q", string(out))
	}
}

// TestVersionFlag_JSONMode_Integration runs the built binary with --output json and
// asserts the JSON envelope is well-formed. This is the Cobra-wiring guarantee —
// flag parsing populates outputFormat before the version template renders.
func TestVersionFlag_JSONMode_Integration(t *testing.T) {
	bin := buildTestBinary(t)
	out, err := exec.Command(bin, "--version", "--output", "json").Output()
	if err != nil {
		t.Fatalf("binary failed: %v", err)
	}
	var parsed map[string]string
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("--version --output json emitted invalid JSON: %v; raw = %q", err, string(out))
	}
	for _, k := range []string{"version", "commit", "built"} {
		if _, ok := parsed[k]; !ok {
			t.Errorf("missing key %q: %v", k, parsed)
		}
	}
	if len(parsed) != 3 {
		t.Errorf("envelope has %d keys, want exactly 3: %v", len(parsed), parsed)
	}
}

// TestVersionFlag_JSONMode_ShortFlagOrder exercises the -o form and the flag-before-version
// ordering. Both should work since pflag is order-independent for flags.
func TestVersionFlag_JSONMode_ShortFlagOrder(t *testing.T) {
	bin := buildTestBinary(t)
	out, err := exec.Command(bin, "-o", "json", "--version").Output()
	if err != nil {
		t.Fatalf("binary failed: %v", err)
	}
	var parsed map[string]string
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("-o json --version emitted invalid JSON: %v; raw = %q", err, string(out))
	}
}

// setVersionVars sets the package-level version vars + outputFormat for a test and
// returns a cleanup function that restores the prior values.
func setVersionVars(t *testing.T, v, c, d, o string) func() {
	t.Helper()
	oldV, oldC, oldD, oldO := version, commit, date, outputFormat
	version, commit, date, outputFormat = v, c, d, o
	return func() { version, commit, date, outputFormat = oldV, oldC, oldD, oldO }
}

// buildTestBinary compiles ./cmd/andamio into a temp binary the test can exec.
// Uses ldflags so the binary reports a recognizable version/commit/date.
func buildTestBinary(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir() + "/andamio-test"
	ldflags := "-X main.version=test -X main.commit=1234567 -X main.date=test-date"
	cmd := exec.Command("go", "build", "-ldflags", ldflags, "-o", tmp, ".")
	if out, err := cmd.CombinedOutput(); err != nil {
		// Missing toolchain or build failure — skip rather than fail so the unit tests
		// still surface useful signal in environments without `go` on PATH.
		if errors.Is(err, exec.ErrNotFound) {
			t.Skip("go toolchain not available for integration build")
		}
		t.Fatalf("build failed: %v\n%s", err, out)
	}
	return tmp
}
