package main

import (
	"fmt"

	"github.com/Andamio-Platform/andamio-cli/internal/config"
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
  - https://mainnet.api.andamio.io (mainnet)`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		url := args[0]

		cfg, err := config.Load()
		if err != nil {
			return err
		}

		cfg.BaseURL = url
		if err := config.Save(cfg); err != nil {
			return err
		}

		fmt.Printf("Base URL set to: %s\n", url)
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

		fmt.Printf("Base URL: %s\n", cfg.BaseURL)
		if cfg.APIKey != "" {
			fmt.Printf("API Key:  %s...\n", cfg.APIKey[:8])
		} else {
			fmt.Println("API Key:  (not set)")
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configSetURLCmd)
	configCmd.AddCommand(configShowCmd)
}
