package config

import (
	"encoding/json"
	"os"
	"path/filepath"
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
			return DefaultConfig(), nil
		}
		return nil, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
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
