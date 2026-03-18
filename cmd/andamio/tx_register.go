package main

import (
	"fmt"
	"os"

	"github.com/Andamio-Platform/andamio-cli/internal/apierr"
	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/Andamio-Platform/andamio-cli/internal/config"
	"github.com/Andamio-Platform/andamio-cli/internal/output"
	"github.com/spf13/cobra"
)

var txRegisterCmd = &cobra.Command{
	Use:   "register",
	Short: "Register a submitted transaction for tracking",
	Long: `Register a submitted transaction hash with the Andamio API for async confirmation tracking.

After submitting a transaction to the Cardano network, register it so the platform
can track its confirmation status.

List valid transaction types with: andamio tx types

Examples:
  andamio tx register --tx-hash abc123... --tx-type access_token_mint
  andamio tx register --tx-hash abc123... --tx-type course_create --instance-id <course-id>`,
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
	RunE: runTxRegister,
}

func init() {
	txCmd.AddCommand(txRegisterCmd)
	txRegisterCmd.Flags().String("tx-hash", "", "Transaction hash (64-character hex)")
	txRegisterCmd.MarkFlagRequired("tx-hash")
	txRegisterCmd.Flags().String("tx-type", "", "Transaction type (see 'andamio tx types')")
	txRegisterCmd.MarkFlagRequired("tx-type")
	txRegisterCmd.Flags().String("instance-id", "", "Course or project ID (optional, for types that return one during build)")
}

func runTxRegister(cmd *cobra.Command, args []string) error {
	txHash, _ := cmd.Flags().GetString("tx-hash")
	txType, _ := cmd.Flags().GetString("tx-type")
	instanceID, _ := cmd.Flags().GetString("instance-id")
	isJSON := output.GetFormat() == output.FormatJSON

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	payload := map[string]interface{}{
		"tx_hash": txHash,
		"tx_type": txType,
	}
	if instanceID != "" {
		payload["instance_id"] = instanceID
	}

	if !isJSON {
		fmt.Fprintf(os.Stderr, "Registering transaction %s (type: %s)\n", txHash, txType)
	}

	c := client.New(cfg)
	var resp map[string]interface{}
	if err := c.Post("/api/v2/tx/register", payload, &resp); err != nil {
		return fmt.Errorf("failed to register transaction: %w", err)
	}

	if isJSON {
		return output.PrintJSON(resp)
	}

	fmt.Fprintf(os.Stderr, "Transaction registered.\n")
	fmt.Fprintf(os.Stderr, "\nNext: andamio tx status %s\n", txHash)
	return nil
}
