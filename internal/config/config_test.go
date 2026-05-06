package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// TestSave_ResetsPermissionsToOnlyOwnerOnOverwrite pins the security
// hardening that prevents config-file secrets (DevJWT, DevRefreshToken,
// UserJWT) from leaking when a previous chmod relaxed the mode.
//
// Background: os.WriteFile applies its mode argument only on file creation;
// on overwrite, it preserves the existing mode. Pre-fix, a config file at
// 0644 stayed at 0644 across saves, which meant the durable 30-day refresh
// token added in #80 PR-A would persist as world-readable. Save now layers
// an explicit os.Chmod(0600) after WriteFile.
func TestSave_ResetsPermissionsToOnlyOwnerOnOverwrite(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix file mode semantics; skip on Windows")
	}

	home := t.TempDir()
	t.Setenv("HOME", home)

	// Pre-create the config file at a permissive mode to simulate a
	// previously-compromised state.
	dir := filepath.Join(home, ".andamio")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte("{}"), 0644); err != nil {
		t.Fatalf("seed permissive config: %v", err)
	}
	// Defensive: if umask interferes, re-chmod explicitly to the
	// world-readable mode the test wants to verify gets reset.
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatalf("chmod permissive: %v", err)
	}

	if err := Save(&Config{DevRefreshToken: "secret"}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat after Save: %v", err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Errorf("after Save: file mode = %#o, want 0600 (Save must restore restrictive perms after WriteFile, which preserves existing mode on overwrite)", got)
	}
}

// TestSave_CreatesAt0600OnFreshInstall covers the file-not-yet-existing
// path. WriteFile honors the mode arg on creation, so 0600 is what we
// expect even before the os.Chmod backstop fires.
func TestSave_CreatesAt0600OnFreshInstall(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix file mode semantics; skip on Windows")
	}

	t.Setenv("HOME", t.TempDir())

	if err := Save(&Config{DevRefreshToken: "secret"}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	path, _ := ConfigPath()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat after Save: %v", err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Errorf("fresh-install file mode = %#o, want 0600", got)
	}
}

// TestLoad_AppliesDevRefreshTokenEnvOverride verifies the
// ANDAMIO_DEV_REFRESH_TOKEN env var injects a refresh token at Load time,
// supporting ephemeral CI/CD agents that want to rotate without writing to
// disk first.
func TestLoad_AppliesDevRefreshTokenEnvOverride(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("ANDAMIO_DEV_REFRESH_TOKEN", "injected.refresh.token")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got, want := cfg.DevRefreshToken, "injected.refresh.token"; got != want {
		t.Errorf("DevRefreshToken = %q, want %q (env var override should populate even when no config file exists)", got, want)
	}
}

// TestLoad_DevRefreshTokenEnvOverridesFileValue covers the file-exists path:
// env var should win over a value already on disk so a CI job can supersede
// a stale token committed by a prior run.
func TestLoad_DevRefreshTokenEnvOverridesFileValue(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := Save(&Config{DevRefreshToken: "stale.from.disk"}); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	t.Setenv("ANDAMIO_DEV_REFRESH_TOKEN", "fresh.from.env")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got, want := cfg.DevRefreshToken, "fresh.from.env"; got != want {
		t.Errorf("DevRefreshToken = %q, want %q (env var must take precedence over file)", got, want)
	}
}

// TestSave_OmitsEnvSourcedRefreshToken pins the documented "ephemeral
// env credentials" contract: a Load that pulled DevRefreshToken from
// ANDAMIO_DEV_REFRESH_TOKEN must NOT write that value to disk on the
// next Save. CI/CD agents inject via env to avoid committing tokens to
// the image; without this contract, the very next successful command
// (which Saves on completion) would persist the env-injected value.
func TestSave_OmitsEnvSourcedRefreshToken(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("ANDAMIO_DEV_REFRESH_TOKEN", "env.injected.MUST-NOT-PERSIST")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := cfg.DevRefreshToken; got != "env.injected.MUST-NOT-PERSIST" {
		t.Fatalf("Load did not apply env override: DevRefreshToken=%q", got)
	}

	// Save while the env value is still in cfg (no rotation yet).
	if err := Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Verify the on-disk JSON does NOT contain the env-injected value.
	path, _ := ConfigPath()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read disk: %v", err)
	}
	if strings.Contains(string(raw), "env.injected.MUST-NOT-PERSIST") {
		t.Errorf("env-sourced DevRefreshToken leaked to disk:\n%s", raw)
	}
	// The dev_refresh_token key itself should be absent (omitempty on the
	// stripped value), not present-with-empty-string.
	var parsed map[string]interface{}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("unmarshal disk: %v", err)
	}
	if _, present := parsed["dev_refresh_token"]; present {
		t.Errorf("dev_refresh_token key present on disk; want absent (env-sourced values must not serialize)")
	}
}

// TestSave_PersistsRotatedRefreshToken_AfterEnvLoad covers the rotation
// case: the agent injects via env, dev refresh runs, the new token from
// the gateway is assigned to cfg.DevRefreshToken, and the new value
// MUST persist (because it's no longer the env-injected one). Without
// this contract, rotation would be invisible to subsequent CLI commands
// in the same job.
func TestSave_PersistsRotatedRefreshToken_AfterEnvLoad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("ANDAMIO_DEV_REFRESH_TOKEN", "env.OLD")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Simulate dev refresh: gateway returns a new token, persistDevSession
	// assigns it. The value now differs from the env snapshot.
	cfg.DevRefreshToken = "rotated.NEW"
	cfg.DevRefreshTokenExpiresAt = "2099-01-01T00:00:00Z"

	if err := Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	path, _ := ConfigPath()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read disk: %v", err)
	}
	if !strings.Contains(string(raw), "rotated.NEW") {
		t.Errorf("rotated refresh token NOT persisted to disk — rotation contract broken:\n%s", raw)
	}
	if strings.Contains(string(raw), "env.OLD") {
		t.Errorf("stale env-sourced token leaked to disk despite rotation:\n%s", raw)
	}
}

// TestSave_OmitsEnvSourcedDevJWT and TestSave_OmitsEnvSourcedUserJWT
// extend the same contract to the other two credential env vars.
func TestSave_OmitsEnvSourcedDevJWT(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("ANDAMIO_DEV_JWT", "env.dev.jwt.MUST-NOT-PERSIST")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	path, _ := ConfigPath()
	raw, _ := os.ReadFile(path)
	if strings.Contains(string(raw), "env.dev.jwt.MUST-NOT-PERSIST") {
		t.Errorf("env-sourced DevJWT leaked to disk:\n%s", raw)
	}
}

func TestSave_OmitsEnvSourcedUserJWT(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("ANDAMIO_JWT", "env.user.jwt.MUST-NOT-PERSIST")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	path, _ := ConfigPath()
	raw, _ := os.ReadFile(path)
	if strings.Contains(string(raw), "env.user.jwt.MUST-NOT-PERSIST") {
		t.Errorf("env-sourced UserJWT leaked to disk:\n%s", raw)
	}
}

// TestSave_PersistsNonCredentialEnvOverrides verifies the deliberate
// asymmetry: ANDAMIO_SUBMIT_URL and ANDAMIO_SUBMIT_HEADERS are
// configuration (not secrets) and should be persisted normally. Only
// the three credential env vars get the ephemeral treatment.
//
// Note: this test seeds an empty config file first because Load only
// applies submit-URL env overrides on the file-exists branch (pre-existing
// quirk, not introduced by this PR). The credential env overrides apply
// on both branches.
func TestSave_PersistsNonCredentialEnvOverrides(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := Save(&Config{}); err != nil {
		t.Fatalf("seed empty config: %v", err)
	}

	t.Setenv("ANDAMIO_SUBMIT_URL", "https://custom.submit.example/api")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	path, _ := ConfigPath()
	raw, _ := os.ReadFile(path)
	if !strings.Contains(string(raw), "custom.submit.example") {
		t.Errorf("non-credential env override (submit URL) NOT persisted; the asymmetry is intentional but the persist path is broken:\n%s", raw)
	}
}

// TestSave_ConcurrentWriters_NoCorruption pins the second failure mode
// closed by atomic write: two concurrent Save calls must not interleave
// bytes and produce malformed JSON. Pre-fix, os.WriteFile's truncate +
// write sequence had no advisory lock and a CI matrix or muscle-memory
// double-shell-run could brick subsequent Loads.
//
// The test spawns N goroutines each calling Save with a unique
// DevRefreshToken value. After all complete, Load must succeed (JSON is
// well-formed) and the persisted value must equal one of the inputs
// (atomic — no partial bytes).
func TestSave_ConcurrentWriters_NoCorruption(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("os.Rename atomicity is POSIX-specific; Windows behavior differs")
	}

	t.Setenv("HOME", t.TempDir())

	const N = 20
	values := make([]string, N)
	for i := range values {
		values[i] = "concurrent.token." + strings.Repeat("x", i+1)
	}

	var wg sync.WaitGroup
	errs := make(chan error, N)
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(v string) {
			defer wg.Done()
			if err := Save(&Config{DevRefreshToken: v}); err != nil {
				errs <- err
			}
		}(values[i])
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent Save returned error: %v", err)
	}

	// Read final state and verify it's well-formed AND its value is one of
	// the inputs (proving atomicity — no byte interleaving).
	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load after concurrent writes: %v (file likely corrupt)", err)
	}
	got := loaded.DevRefreshToken
	found := false
	for _, want := range values {
		if got == want {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("DevRefreshToken = %q after concurrent writes; want one of the input values (mismatch indicates byte interleaving)", got)
	}
}

// TestSave_NoTempFileLeftBehindOnSuccess pins that the atomic-write
// tempfile is renamed onto the canonical path on success, leaving no
// stray `.config.json.tmp.*` files in the .andamio directory.
func TestSave_NoTempFileLeftBehindOnSuccess(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if err := Save(&Config{DevRefreshToken: "x"}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	path, _ := ConfigPath()
	dir := filepath.Dir(path)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".config.json.tmp.") {
			t.Errorf("tempfile leaked: %s (atomic write should rename onto canonical path)", e.Name())
		}
	}
}
