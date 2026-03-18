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
}

// ClearUserAuth removes all user authentication fields from the config.
func (c *Config) ClearUserAuth() {
	c.UserJWT = ""
	c.JWTExpiresAt = ""
	c.UserAlias = ""
	c.UserID = ""
}

// HasUserAuth returns true if the config has a user JWT stored.
func (c *Config) HasUserAuth() bool {
	return c.UserJWT != ""
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
