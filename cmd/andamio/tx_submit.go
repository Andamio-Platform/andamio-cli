package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/Andamio-Platform/andamio-cli/internal/config"
	"github.com/Andamio-Platform/andamio-cli/internal/output"
	"github.com/Andamio-Platform/andamio-cli/internal/submit"
	"github.com/spf13/cobra"
)

var txSubmitCmd = &cobra.Command{
	Use:   "submit",
	Short: "Submit a signed transaction to the Cardano network",
	Long: `Submit a signed Cardano transaction to the network via a submit API.

The submit URL can be provided via --submit-url flag or configured with:
  andamio config set-submit-url <url>

Custom headers (e.g., for Blockfrost API key) can be passed with --submit-header.

Examples:
  andamio tx submit --tx 84a4... --submit-url https://submit.example.com --output json
  andamio tx submit --tx-file signed.cbor --submit-header "project_id: preprodABC123"`,
	RunE: runTxSubmit,
}

func init() {
	txCmd.AddCommand(txSubmitCmd)
	txSubmitCmd.Flags().String("tx", "", "Signed transaction CBOR hex string")
	txSubmitCmd.Flags().String("tx-file", "", "Path to file containing signed CBOR hex (mutually exclusive with --tx)")
	txSubmitCmd.Flags().String("submit-url", "", "Cardano submit API URL (falls back to config)")
	txSubmitCmd.Flags().StringArray("submit-header", nil, "Additional HTTP header (repeatable, format: \"Key: Value\")")
}

// mergeSubmitHeaders merges config-level headers with flag-level headers.
// Flag headers override config headers with the same key (case-insensitive key match).
// Returns a merged []string slice of "Key: Value" strings.
func mergeSubmitHeaders(configHeaders map[string]string, flagHeaders []string) []string {
	// Build a map of config headers keyed by lowercase key
	merged := make(map[string]string) // lowercase key -> "Key: Value"
	for k, v := range configHeaders {
		lower := strings.ToLower(k)
		merged[lower] = k + ": " + v
	}

	// Flag headers override config headers with the same key
	for _, h := range flagHeaders {
		parts := strings.SplitN(h, ":", 2)
		if len(parts) != 2 {
			// Pass through malformed headers as-is; submit package will handle them
			merged[strings.ToLower(strings.TrimSpace(h))] = h
			continue
		}
		key := strings.TrimSpace(parts[0])
		lower := strings.ToLower(key)
		merged[lower] = key + ": " + strings.TrimSpace(parts[1])
	}

	result := make([]string, 0, len(merged))
	for _, v := range merged {
		result = append(result, v)
	}
	return result
}

func runTxSubmit(cmd *cobra.Command, args []string) error {
	txHex, _ := cmd.Flags().GetString("tx")
	txFile, _ := cmd.Flags().GetString("tx-file")
	submitURL, _ := cmd.Flags().GetString("submit-url")
	flagHeaders, _ := cmd.Flags().GetStringArray("submit-header")
	isJSON := output.GetFormat() == output.FormatJSON

	if txHex != "" && txFile != "" {
		return fmt.Errorf("--tx and --tx-file are mutually exclusive")
	}
	if txHex == "" && txFile == "" {
		return fmt.Errorf("either --tx or --tx-file is required")
	}

	// Load tx from file if needed
	if txFile != "" {
		data, err := os.ReadFile(txFile)
		if err != nil {
			return fmt.Errorf("failed to read tx file: %w", err)
		}
		txHex = strings.TrimSpace(string(data))
	}

	// Resolve submit URL and config headers: flag > config
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if submitURL == "" {
		submitURL = cfg.SubmitURL
	}
	if submitURL == "" {
		return fmt.Errorf("no submit URL configured\n\nSet one with:\n  andamio config set-submit-url <url>\n\nOr pass --submit-url <url>")
	}

	if err := config.ValidateSubmitURL(submitURL); err != nil {
		return err
	}

	// Merge config headers with flag headers (flag headers take precedence)
	headers := mergeSubmitHeaders(cfg.SubmitHeaders, flagHeaders)

	if !isJSON {
		fmt.Fprintf(os.Stderr, "Submitting transaction to %s\n", submitURL)
	}

	respBody, err := submit.SubmitTransaction(submitURL, txHex, headers)
	if err != nil {
		return err
	}

	// Try to parse response as JSON, otherwise wrap in an object
	var respData interface{}
	if err := json.Unmarshal(respBody, &respData); err != nil {
		// Response is not JSON (e.g., plain tx hash from cardano-submit-api)
		txHash := strings.TrimSpace(string(respBody))
		respData = map[string]string{"tx_hash": txHash}
	}

	if isJSON {
		return output.PrintJSON(respData)
	}

	// Text mode
	if m, ok := respData.(map[string]interface{}); ok {
		if txHash, ok := m["tx_hash"].(string); ok {
			fmt.Printf("TX Hash: %s\n", txHash)
		}
	} else if s, ok := respData.(string); ok {
		fmt.Printf("TX Hash: %s\n", s)
	}
	fmt.Fprintf(os.Stderr, "\nNext: andamio tx register --tx-hash <hash> --tx-type <type>\n")
	return nil
}
