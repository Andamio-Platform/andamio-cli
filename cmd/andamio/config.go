package main

import (
	"fmt"

	"github.com/Andamio-Platform/andamio-cli/internal/config"
	"github.com/Andamio-Platform/andamio-cli/internal/output"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage CLI configuration",
}

var configSetURLCmd = &cobra.Command{
	Use:   "set-url [url]",
	Short: "Set the API base URL",
	Long: `Set the API base URL. Common values:
  - https://preprod.api.andamio.io (preprod, default)
  - https://mainnet.api.andamio.io (mainnet)

Set ANDAMIO_ALLOW_ANY_URL=1 to allow non-andamio.io URLs for testing.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		rawURL := args[0]

		// Use shared URL validation (supports ANDAMIO_ALLOW_ANY_URL override)
		if err := config.ValidateBaseURL(rawURL); err != nil {
			return err
		}

		cfg, err := config.Load()
		if err != nil {
			return err
		}

		cfg.BaseURL = rawURL
		if err := config.Save(cfg); err != nil {
			return err
		}

		fmt.Printf("Base URL set to: %s\n", rawURL)
		return nil
	},
}

var configSetSubmitURLCmd = &cobra.Command{
	Use:   "set-submit-url [url]",
	Short: "Set the Cardano submit API URL",
	Long: `Set the URL for submitting signed transactions to the Cardano network.

This can be any Cardano submit API endpoint (Blockfrost, Maestro, self-hosted, etc.).
Requires HTTPS for non-localhost URLs. Set ANDAMIO_ALLOW_ANY_URL=1 to bypass.

Examples:
  andamio config set-submit-url https://cardano-mainnet.blockfrost.io/api/tx/submit
  andamio config set-submit-url https://submit-api.example.com`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		rawURL := args[0]

		if err := config.ValidateSubmitURL(rawURL); err != nil {
			return err
		}

		cfg, err := config.Load()
		if err != nil {
			return err
		}

		cfg.SubmitURL = rawURL
		if err := config.Save(cfg); err != nil {
			return err
		}

		fmt.Printf("Submit URL set to: %s\n", rawURL)
		return nil
	},
}

var configSetSubmitHeaderCmd = &cobra.Command{
	Use:   "set-submit-header [key] [value]",
	Short: "Set a submit API header (e.g., Blockfrost project_id)",
	Long: `Set a persistent HTTP header for the Cardano submit API.

Headers are sent with every tx submit/tx run invocation. Useful for
API keys required by providers like Blockfrost or Maestro.

Flag-level --submit-header values override config headers with the same key.

Examples:
  andamio config set-submit-header project_id preprodABC123
  andamio config set-submit-header Authorization "Bearer tok_xyz"`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		key := args[0]
		value := args[1]

		cfg, err := config.Load()
		if err != nil {
			return err
		}

		if cfg.SubmitHeaders == nil {
			cfg.SubmitHeaders = make(map[string]string)
		}
		cfg.SubmitHeaders[key] = value

		if err := config.Save(cfg); err != nil {
			return err
		}

		fmt.Printf("Submit header set: %s\n", key)
		return nil
	},
}

var configRemoveSubmitHeaderCmd = &cobra.Command{
	Use:   "remove-submit-header [key]",
	Short: "Remove a submit API header",
	Long: `Remove a persistent HTTP header for the Cardano submit API.

Examples:
  andamio config remove-submit-header project_id`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		key := args[0]

		cfg, err := config.Load()
		if err != nil {
			return err
		}

		if _, exists := cfg.SubmitHeaders[key]; cfg.SubmitHeaders == nil || !exists {
			fmt.Printf("Submit header %q not found in config\n", key)
			return nil
		}

		delete(cfg.SubmitHeaders, key)

		if err := config.Save(cfg); err != nil {
			return err
		}

		fmt.Printf("Submit header removed: %s\n", key)
		return nil
	},
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show current configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}

		if output.GetFormat() == output.FormatJSON {
			type configStatus struct {
				BaseURL       string            `json:"base_url"`
				APIKeySet     bool              `json:"api_key_set"`
				SubmitURL     string            `json:"submit_url,omitempty"`
				SubmitHeaders map[string]string `json:"submit_headers,omitempty"`
			}
			return output.PrintJSON(configStatus{
				BaseURL:       cfg.BaseURL,
				APIKeySet:     cfg.APIKey != "",
				SubmitURL:     cfg.SubmitURL,
				SubmitHeaders: cfg.SubmitHeaders,
			})
		}

		fmt.Printf("Base URL:   %s\n", cfg.BaseURL)
		if cfg.APIKey != "" {
			fmt.Println("API Key:    ****... (configured)")
		} else {
			fmt.Println("API Key:    (not set)")
		}
		if cfg.SubmitURL != "" {
			fmt.Printf("Submit URL: %s\n", cfg.SubmitURL)
		} else {
			fmt.Println("Submit URL: (not set)")
		}
		if len(cfg.SubmitHeaders) > 0 {
			fmt.Println("Submit Headers:")
			for k, v := range cfg.SubmitHeaders {
				masked := v
				if len(v) > 4 {
					masked = v[:4] + "..."
				}
				fmt.Printf("  %s: %s\n", k, masked)
			}
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configSetURLCmd)
	configCmd.AddCommand(configSetSubmitURLCmd)
	configCmd.AddCommand(configSetSubmitHeaderCmd)
	configCmd.AddCommand(configRemoveSubmitHeaderCmd)
	configCmd.AddCommand(configShowCmd)
}
