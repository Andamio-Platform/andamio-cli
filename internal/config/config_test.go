package config

import (
	"os"
	"path/filepath"
	"runtime"
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
