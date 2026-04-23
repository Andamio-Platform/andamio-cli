package main

import (
	"context"

	"github.com/Andamio-Platform/andamio-cli/internal/apierr"
	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/Andamio-Platform/andamio-cli/internal/config"
	"github.com/Andamio-Platform/andamio-cli/internal/output"
	"github.com/spf13/cobra"
)

var apikeyCmd = &cobra.Command{
	Use:   "apikey",
	Short: "API key management",
}

var apikeyUsageCmd = &cobra.Command{
	Use:   "usage",
	Short: "Get API key usage stats",
	RunE: func(cmd *cobra.Command, args []string) error {
		return getAPIKeyJSON(cmd.Context(), "/api/v2/apikey/developer/usage/get")
	},
}

var apikeyProfileCmd = &cobra.Command{
	Use:   "profile",
	Short: "Get API key profile",
	RunE: func(cmd *cobra.Command, args []string) error {
		return getAPIKeyJSON(cmd.Context(), "/api/v2/apikey/developer/profile/get")
	},
}

func init() {
	rootCmd.AddCommand(apikeyCmd)
	apikeyCmd.AddCommand(apikeyUsageCmd)
	apikeyCmd.AddCommand(apikeyProfileCmd)
}

// getAPIKeyJSON sends a GET request using only API key auth (no wallet JWT).
// The /v2/apikey/developer/* endpoints reject wallet JWTs, so we must not
// send the Authorization header even when a wallet session exists.
func getAPIKeyJSON(ctx context.Context, path string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.APIKey == "" {
		return &apierr.AuthError{
			Message: "apikey commands require an API key. Run 'andamio auth login --api-key <key>'",
		}
	}
	// Copy config and clear wallet JWT so only X-API-Key is sent
	apiKeyCfg := *cfg
	apiKeyCfg.UserJWT = ""
	c := client.New(&apiKeyCfg)
	var result map[string]interface{}
	if err := c.Get(ctx, path, &result); err != nil {
		return err
	}
	return output.PrintJSON(result)
}
