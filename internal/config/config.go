package config

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	APIKey       string `json:"api_key"`
	BaseURL      string `json:"base_url"`
	UserJWT      string `json:"user_jwt,omitempty"`
	JWTExpiresAt string `json:"jwt_expires_at,omitempty"`
	UserAlias    string `json:"user_alias,omitempty"`
	UserID       string `json:"user_id,omitempty"`
	UserKeyHash  string `json:"user_key_hash,omitempty"`
	// Developer-portal auth — RS256 JWT minted by the gateway's CIP-30
	// signature-verified developer login (`POST /v2/auth/developer/login/...`)
	// and required for /v2/keys + future developer-scoped endpoints. Stored
	// alongside (not replacing) the wallet/user JWT above; the two cover
	// distinct gateway middlewares. JWT lifetime is short (60 min as of
	// andamio-api #410) so the refresh token below is the durable credential
	// — `dev refresh` rotates it. See `andamio dev login`.
	DevJWT                   string            `json:"dev_jwt,omitempty"`
	DevJWTExpiresAt          string            `json:"dev_jwt_expires_at,omitempty"`
	DevAlias                 string            `json:"dev_alias,omitempty"`
	DevID                    string            `json:"dev_id,omitempty"`
	DevKeyHash               string            `json:"dev_key_hash,omitempty"`
	DevTier                  string            `json:"dev_tier,omitempty"`
	DevRefreshToken          string            `json:"dev_refresh_token,omitempty"`
	DevRefreshTokenExpiresAt string            `json:"dev_refresh_token_expires_at,omitempty"`
	SubmitURL                string            `json:"submit_url,omitempty"`
	SubmitHeaders            map[string]string `json:"submit_headers,omitempty"`

	// envInjected captures values pulled from environment variables at
	// Load time. Save() consults this snapshot to omit env-sourced values
	// that haven't been rotated since Load — env-injected credentials are
	// documented as ephemeral so a CI/CD agent can inject `ANDAMIO_DEV_*`
	// without polluting the on-disk config. When code mutates a field
	// (e.g., persistDevSession after a successful refresh) the new value
	// differs from the snapshot and is persisted normally — rotation
	// keeps working as designed.
	//
	// Unexported, so Go's json package skips it on Marshal AND Unmarshal
	// — no `json:"-"` tag needed (it would only matter on exported fields).
	envInjected envSnapshot
}

// envSnapshot records the credential values pulled from environment
// variables at Load time. Used by Save to distinguish "still env-sourced"
// from "rotated since Load and should persist." Fields are intentionally
// only the credential-bearing ones (DevRefreshToken, DevJWT, UserJWT) —
// non-secret env overrides (ANDAMIO_SUBMIT_URL, ANDAMIO_SUBMIT_HEADERS)
// are persisted normally because they're configuration, not secrets.
type envSnapshot struct {
	DevRefreshToken string
	DevJWT          string
	UserJWT         string
}

// ClearUserAuth removes all user authentication fields from the config.
func (c *Config) ClearUserAuth() {
	c.UserJWT = ""
	c.JWTExpiresAt = ""
	c.UserAlias = ""
	c.UserID = ""
	c.UserKeyHash = ""
}

// HasUserAuth returns true if the config has a user JWT stored.
func (c *Config) HasUserAuth() bool {
	return c.UserJWT != ""
}

// ClearDevAuth removes all developer-portal authentication fields. Mirrors
// ClearUserAuth — the two clear independent slots so `dev logout` does not
// disturb wallet/user sessions and vice versa. Includes the refresh token so
// logout fully unbinds the device; subsequent `dev refresh` will fail and
// `dev login` is required.
func (c *Config) ClearDevAuth() {
	c.DevJWT = ""
	c.DevJWTExpiresAt = ""
	c.DevAlias = ""
	c.DevID = ""
	c.DevKeyHash = ""
	c.DevTier = ""
	c.DevRefreshToken = ""
	c.DevRefreshTokenExpiresAt = ""
}

// HasDevAuth returns true if the config has a developer JWT stored.
func (c *Config) HasDevAuth() bool {
	return c.DevJWT != ""
}

func DefaultConfig() *Config {
	return &Config{
		BaseURL: "https://preprod.api.andamio.io",
	}
}

// ValidateBaseURL checks if the URL is safe to use.
// Returns nil if valid, error if invalid.
// Set ANDAMIO_ALLOW_ANY_URL=1 to bypass validation for automation/testing.
func ValidateBaseURL(rawURL string) error {
	// Allow bypass for automation/CI scenarios
	if os.Getenv("ANDAMIO_ALLOW_ANY_URL") == "1" {
		return nil
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	hostname := parsed.Hostname()

	// Allow localhost for development (including IPv6)
	isLocalhost := hostname == "localhost" || hostname == "127.0.0.1" || hostname == "::1"
	if isLocalhost {
		return nil
	}

	// Require HTTPS for non-localhost
	if parsed.Scheme != "https" {
		return fmt.Errorf("URL must use HTTPS (got %s)", parsed.Scheme)
	}

	// Validate domain - must be andamio.io or a subdomain of it
	// Check for exact match OR subdomain (with leading dot to prevent lookalikes)
	if hostname != "andamio.io" && !strings.HasSuffix(hostname, ".andamio.io") {
		return fmt.Errorf("URL must be an andamio.io domain or localhost (got %s)", hostname)
	}

	return nil
}

// ValidateSubmitURL checks if a Cardano submit API URL is safe to use.
// Unlike ValidateBaseURL, this allows any domain (submit APIs are third-party services).
// Requires HTTPS for non-localhost. Set ANDAMIO_ALLOW_ANY_URL=1 to bypass.
func ValidateSubmitURL(rawURL string) error {
	if os.Getenv("ANDAMIO_ALLOW_ANY_URL") == "1" {
		return nil
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	hostname := parsed.Hostname()
	isLocalhost := hostname == "localhost" || hostname == "127.0.0.1" || hostname == "::1"
	if isLocalhost {
		return nil
	}

	if parsed.Scheme != "https" {
		return fmt.Errorf("submit URL must use HTTPS (got %s)", parsed.Scheme)
	}

	return nil
}

// applyCredentialEnvOverrides reads ANDAMIO_JWT, ANDAMIO_DEV_JWT, and
// ANDAMIO_DEV_REFRESH_TOKEN from the environment and applies them to cfg
// AND records the original env values in cfg.envInjected. The snapshot is
// the load-bearing piece for the "ephemeral env credentials" contract —
// Save consults it to decide whether each field's current value still
// matches what env injected (omit on save) or has since been rotated by
// code (persist normally).
//
// Non-credential env overrides (ANDAMIO_SUBMIT_URL, ANDAMIO_SUBMIT_HEADERS)
// are intentionally NOT tracked — they're configuration, not secrets, and
// persisting them is the documented behavior. They're handled inline in
// Load() below.
func applyCredentialEnvOverrides(cfg *Config) {
	if jwt := os.Getenv("ANDAMIO_JWT"); jwt != "" {
		cfg.UserJWT = jwt
		cfg.envInjected.UserJWT = jwt
	}
	if devJWT := os.Getenv("ANDAMIO_DEV_JWT"); devJWT != "" {
		cfg.DevJWT = devJWT
		cfg.envInjected.DevJWT = devJWT
	}
	if devRefresh := os.Getenv("ANDAMIO_DEV_REFRESH_TOKEN"); devRefresh != "" {
		cfg.DevRefreshToken = devRefresh
		cfg.envInjected.DevRefreshToken = devRefresh
	}
}

func ConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".andamio", "config.json"), nil
}

func Load() (*Config, error) {
	path, err := ConfigPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			cfg := DefaultConfig()
			applyCredentialEnvOverrides(cfg)
			return cfg, nil
		}
		return nil, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	applyCredentialEnvOverrides(&cfg)

	// ANDAMIO_SUBMIT_URL env var overrides stored submit URL
	if submitURL := os.Getenv("ANDAMIO_SUBMIT_URL"); submitURL != "" {
		cfg.SubmitURL = submitURL
	}

	// ANDAMIO_SUBMIT_HEADERS env var overrides stored submit headers (JSON map)
	if headersJSON := os.Getenv("ANDAMIO_SUBMIT_HEADERS"); headersJSON != "" {
		var headers map[string]string
		if err := json.Unmarshal([]byte(headersJSON), &headers); err != nil {
			return nil, fmt.Errorf("invalid ANDAMIO_SUBMIT_HEADERS (expected JSON object): %w", err)
		}
		cfg.SubmitHeaders = headers
	}

	// Validate base URL on load to catch config file tampering
	if cfg.BaseURL != "" {
		if err := ValidateBaseURL(cfg.BaseURL); err != nil {
			return nil, fmt.Errorf("invalid base URL in config: %w (set ANDAMIO_ALLOW_ANY_URL=1 to override)", err)
		}
	}

	return &cfg, nil
}

func Save(cfg *Config) error {
	path, err := ConfigPath()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}

	// Strip env-sourced credentials before serializing so the documented
	// "ephemeral" contract holds: env-injected values stay in process
	// memory only, not written to disk on every successful save. When a
	// field is mutated by code (e.g., persistDevSession after a successful
	// refresh) the new value differs from the snapshot and is persisted
	// normally — so rotation keeps working.
	serialized := *cfg
	if cfg.envInjected.DevRefreshToken != "" && serialized.DevRefreshToken == cfg.envInjected.DevRefreshToken {
		serialized.DevRefreshToken = ""
		serialized.DevRefreshTokenExpiresAt = ""
	}
	if cfg.envInjected.DevJWT != "" && serialized.DevJWT == cfg.envInjected.DevJWT {
		serialized.DevJWT = ""
		serialized.DevJWTExpiresAt = ""
	}
	if cfg.envInjected.UserJWT != "" && serialized.UserJWT == cfg.envInjected.UserJWT {
		serialized.UserJWT = ""
		serialized.JWTExpiresAt = ""
	}

	data, err := json.MarshalIndent(&serialized, "", "  ")
	if err != nil {
		return err
	}

	return atomicWriteSecret(path, data)
}

// atomicWriteSecret writes data to path via a sibling tempfile + rename,
// chmod'd to 0600 before any bytes hit disk. Compared to the prior
// os.WriteFile + os.Chmod sequence, this closes two failure modes:
//
//  1. TOCTOU window. os.WriteFile honors its mode arg only on file
//     creation; on overwrite, secret bytes touch disk under the existing
//     mode (potentially 0644 from a prior buggy save, a manual chmod, or
//     a non-Unix backup restore) before a follow-up chmod tightens.
//     A co-tenant polling the path during that microsecond window can
//     read the freshly-written secrets.
//
//  2. Concurrent-writer corruption. WriteFile's underlying syscalls are
//     truncate + write with no advisory lock. Two concurrent Save calls
//     (e.g., parallel `dev refresh` shells, or a CI matrix job) can
//     interleave bytes and produce malformed JSON, locking subsequent
//     Loads out until the user manually deletes the file.
//
// os.Rename is atomic on POSIX same-filesystem and inherits the
// tempfile's 0600 mode. On rename failure, the tempfile is cleaned up so
// a successful prior save remains on disk and a subsequent save retries
// from a clean slate.
//
// The tempfile is created via os.CreateTemp (atomic O_CREAT|O_EXCL with
// a randomized name) at mode 0600; an explicit chmod follows as a
// belt-and-braces guard against unusual filesystem semantics.
func atomicWriteSecret(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".config.json.tmp.*")
	if err != nil {
		return fmt.Errorf("create tempfile: %w", err)
	}
	tmpPath := tmp.Name()

	// Track success so the deferred cleanup only fires on error paths.
	// On success we must NOT remove the tempfile because os.Rename has
	// already moved it onto the canonical path.
	success := false
	defer func() {
		if !success {
			_ = os.Remove(tmpPath)
		}
	}()

	if err := os.Chmod(tmpPath, 0600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod tempfile: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write tempfile: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync tempfile: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close tempfile: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("rename tempfile: %w", err)
	}

	success = true
	return nil
}
