package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/Andamio-Platform/andamio-cli/internal/config"
	"github.com/Andamio-Platform/andamio-cli/internal/output"
	"github.com/spf13/cobra"
)

var tokenCmd = &cobra.Command{
	Use:   "token",
	Short: "Native asset token registry",
}

var tokenListCmd = &cobra.Command{
	Use:   "list",
	Short: "List registered tokens available as task rewards",
	Long: `List all registered native asset tokens from the global token registry.

These are the tokens that can be attached to project tasks via --token flag.

Use the policy_id and asset_name values with project task create/update:
  andamio project task create <project-id> --title "..." --lovelace 5000000 \
    --expiration 2026-06-01 --token "policy_id,asset_name,quantity"`,
	RunE: runTokenList,
}

func init() {
	rootCmd.AddCommand(tokenCmd)
	tokenCmd.AddCommand(tokenListCmd)
}

func runTokenList(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	c := client.New(cfg)

	// API may return {data: [...]} envelope or a raw array
	var raw json.RawMessage
	if err := c.Get(cmd.Context(), "/api/v2/token/user/tokens/list", &raw); err != nil {
		return fmt.Errorf("failed to list tokens: %w", err)
	}

	// Normalize to {data: [...]} envelope
	var data []interface{}
	var envelope map[string]interface{}
	if json.Unmarshal(raw, &envelope) == nil {
		data, _ = envelope["data"].([]interface{})
	} else if json.Unmarshal(raw, &data) == nil {
		envelope = map[string]interface{}{"data": data}
	} else {
		return fmt.Errorf("unexpected token list response format")
	}

	if output.GetFormat() == output.FormatJSON {
		return output.PrintJSON(envelope)
	}

	if len(data) == 0 {
		fmt.Fprintln(os.Stderr, "No registered tokens found.")
		return nil
	}

	// Text table output
	fmt.Printf("%-12s %-58s %-20s %s\n", "Ticker", "Policy ID", "Asset Name", "Decimals")
	fmt.Printf("%-12s %-58s %-20s %s\n", "------", "---------", "----------", "--------")

	for _, item := range data {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		policyID, _ := m["policy_id"].(string)
		assetNameDecoded, _ := m["asset_name_decoded"].(string)
		ticker := ""
		if t, ok := m["ticker"].(string); ok {
			ticker = t
		}
		decimals := 0
		if d, ok := m["decimals"].(float64); ok {
			decimals = int(d)
		}

		if ticker == "" {
			ticker = assetNameDecoded
		}

		fmt.Printf("%-12s %-58s %-20s %d\n", ticker, policyID, assetNameDecoded, decimals)
	}

	return nil
}
