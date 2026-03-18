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
				BaseURL   string `json:"base_url"`
				APIKeySet bool   `json:"api_key_set"`
				SubmitURL string `json:"submit_url,omitempty"`
			}
			return output.PrintJSON(configStatus{
				BaseURL:   cfg.BaseURL,
				APIKeySet: cfg.APIKey != "",
				SubmitURL: cfg.SubmitURL,
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
		return nil
	},
}

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configSetURLCmd)
	configCmd.AddCommand(configSetSubmitURLCmd)
	configCmd.AddCommand(configShowCmd)
}
