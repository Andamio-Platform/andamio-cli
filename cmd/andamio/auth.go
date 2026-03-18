package main

import (
	"fmt"

	"github.com/Andamio-Platform/andamio-cli/internal/config"
	"github.com/Andamio-Platform/andamio-cli/internal/output"
	"github.com/spf13/cobra"
)

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Authenticate with the Andamio API",
	Long:  `Store your API key for authenticating with Andamio endpoints.`,
}

var authLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Store API key",
	RunE: func(cmd *cobra.Command, args []string) error {
		apiKey, _ := cmd.Flags().GetString("api-key")
		if apiKey == "" {
			return fmt.Errorf("--api-key is required")
		}

		cfg, err := config.Load()
		if err != nil {
			return err
		}

		cfg.APIKey = apiKey
		if err := config.Save(cfg); err != nil {
			return err
		}

		fmt.Println("API key saved to ~/.andamio/config.json")
		return nil
	},
}

var authStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check authentication status",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}

		if output.GetFormat() == output.FormatJSON {
			type authStatus struct {
				APIKeySet bool   `json:"api_key_set"`
				BaseURL   string `json:"base_url"`
			}
			return output.PrintJSON(authStatus{
				APIKeySet: cfg.APIKey != "",
				BaseURL:   cfg.BaseURL,
			})
		}

		if cfg.APIKey == "" {
			fmt.Println("Not authenticated. Run: andamio auth login --api-key <key>")
		} else {
			fmt.Println("Authenticated (API key configured)")
			fmt.Printf("Base URL: %s\n", cfg.BaseURL)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(authCmd)
	authCmd.AddCommand(authLoginCmd)
	authCmd.AddCommand(authStatusCmd)

	authLoginCmd.Flags().String("api-key", "", "Your Andamio API key")
}
