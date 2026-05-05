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
			if jwt := os.Getenv("ANDAMIO_JWT"); jwt != "" {
				cfg.UserJWT = jwt
			}
			if devJWT := os.Getenv("ANDAMIO_DEV_JWT"); devJWT != "" {
				cfg.DevJWT = devJWT
			}
			return cfg, nil
		}
		return nil, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	// ANDAMIO_JWT env var overrides stored JWT (for CI/CD and headless environments)
	if jwt := os.Getenv("ANDAMIO_JWT"); jwt != "" {
		cfg.UserJWT = jwt
	}

	// ANDAMIO_DEV_JWT env var overrides stored developer JWT, parallel to
	// ANDAMIO_JWT. Required by `dev keys` operations and other developer-
	// portal endpoints; distinct from ANDAMIO_JWT which targets wallet-scoped
	// commands.
	if devJWT := os.Getenv("ANDAMIO_DEV_JWT"); devJWT != "" {
		cfg.DevJWT = devJWT
	}

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

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0600)
}
