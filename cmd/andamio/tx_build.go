package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/Andamio-Platform/andamio-cli/internal/apierr"
	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/Andamio-Platform/andamio-cli/internal/config"
	"github.com/Andamio-Platform/andamio-cli/internal/output"
	"github.com/spf13/cobra"
)

var txBuildCmd = &cobra.Command{
	Use:   "build <endpoint>",
	Short: "Build an unsigned transaction via the API",
	Long: `POST to an Andamio API transaction-building endpoint and return the unsigned transaction.

The endpoint should be a /v2/tx/ path. The request body is passed via --body (inline JSON)
or --body-file (path to JSON file).

Returns the full API response including unsigned_tx and any endpoint-specific fields.

List available transaction types with: andamio tx types

Examples:
  andamio tx build /v2/tx/global/user/access-token/mint --body '{"alias":"dev1","initiator_data":"addr_test1..."}'
  andamio tx build /v2/tx/instance/owner/course/create --body-file create-course.json --output json`,
	Args: cobra.ExactArgs(1),
	PreRunE: func(cmd *cobra.Command, args []string) error {
		if err := rootCmd.PersistentPreRunE(cmd, args); err != nil {
			return err
		}
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		if !cfg.HasUserAuth() {
			return &apierr.AuthError{Message: "not authenticated. Run 'andamio user login' first"}
		}
		return nil
	},
	RunE: runTxBuild,
}

func init() {
	txCmd.AddCommand(txBuildCmd)
	txBuildCmd.Flags().String("body", "", "Inline JSON request body")
	txBuildCmd.Flags().String("body-file", "", "Path to JSON file (mutually exclusive with --body)")
}

func runTxBuild(cmd *cobra.Command, args []string) error {
	endpoint := args[0]
	bodyStr, _ := cmd.Flags().GetString("body")
	bodyFile, _ := cmd.Flags().GetString("body-file")
	isJSON := output.GetFormat() == output.FormatJSON

	if bodyStr == "" && bodyFile == "" {
		return fmt.Errorf("either --body or --body-file is required")
	}
	if bodyStr != "" && bodyFile != "" {
		return fmt.Errorf("--body and --body-file are mutually exclusive")
	}

	// Load body
	var bodyData interface{}
	if bodyFile != "" {
		data, err := os.ReadFile(bodyFile)
		if err != nil {
			return fmt.Errorf("failed to read body file: %w", err)
		}
		if err := json.Unmarshal(data, &bodyData); err != nil {
			return fmt.Errorf("invalid JSON in body file: %w", err)
		}
	} else if bodyStr != "" {
		if err := json.Unmarshal([]byte(bodyStr), &bodyData); err != nil {
			return fmt.Errorf("invalid JSON in --body: %w", err)
		}
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	if !isJSON {
		fmt.Fprintf(os.Stderr, "Building transaction: POST %s\n", endpoint)
	}

	c := client.New(cfg)
	var resp map[string]interface{}
	if err := c.Post("/api"+endpoint, bodyData, &resp); err != nil {
		return fmt.Errorf("failed to build transaction: %w", err)
	}

	if isJSON {
		return output.PrintJSON(resp)
	}

	// Text mode: show key fields
	if unsignedTx, ok := resp["unsigned_tx"].(string); ok {
		if len(unsignedTx) > 32 {
			fmt.Printf("Unsigned TX: %s...%s\n", unsignedTx[:16], unsignedTx[len(unsignedTx)-16:])
		} else {
			fmt.Printf("Unsigned TX: %s\n", unsignedTx)
		}
	}
	for _, key := range []string{"course_id", "project_id"} {
		if v, ok := resp[key].(string); ok {
			fmt.Printf("%s: %s\n", key, v)
		}
	}
	fmt.Fprintf(os.Stderr, "\nNext: andamio tx sign --tx <unsigned_tx> --skey <path>\n")
	return nil
}
