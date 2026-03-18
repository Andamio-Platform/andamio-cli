package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/Andamio-Platform/andamio-cli/internal/cardano"
	"github.com/Andamio-Platform/andamio-cli/internal/output"
	"github.com/spf13/cobra"
)

var txSignCmd = &cobra.Command{
	Use:   "sign",
	Short: "Sign an unsigned transaction with a local .skey file",
	Long: `Sign an unsigned Cardano transaction locally using a .skey file.

The unsigned transaction CBOR hex can be provided via --tx flag or --tx-file.
The signing key is loaded via Bursa (cardano-cli JSON envelope format).

Outputs the signed transaction CBOR hex and transaction hash.

Examples:
  andamio tx sign --tx 84a4... --skey ./payment.skey --output json
  andamio tx sign --tx-file unsigned.cbor --skey ./payment.skey --output json`,
	RunE: runTxSign,
}

func init() {
	txCmd.AddCommand(txSignCmd)
	txSignCmd.Flags().String("tx", "", "Unsigned transaction CBOR hex string")
	txSignCmd.Flags().String("tx-file", "", "Path to file containing CBOR hex (mutually exclusive with --tx)")
	txSignCmd.Flags().String("skey", "", "Path to .skey file (cardano-cli JSON envelope format)")
	txSignCmd.MarkFlagRequired("skey")
}

func runTxSign(cmd *cobra.Command, args []string) error {
	txHex, _ := cmd.Flags().GetString("tx")
	txFile, _ := cmd.Flags().GetString("tx-file")
	skeyPath, _ := cmd.Flags().GetString("skey")
	isJSON := output.GetFormat() == output.FormatJSON

	if txHex != "" && txFile != "" {
		return fmt.Errorf("--tx and --tx-file are mutually exclusive")
	}
	if txHex == "" && txFile == "" {
		return fmt.Errorf("either --tx or --tx-file is required")
	}

	// Load unsigned tx
	if txFile != "" {
		data, err := os.ReadFile(txFile)
		if err != nil {
			return fmt.Errorf("failed to read tx file: %w", err)
		}
		txHex = strings.TrimSpace(string(data))
	}

	if !isJSON {
		fmt.Fprintf(os.Stderr, "Loading signing key: %s\n", skeyPath)
	}

	// Load key
	privKey, pubKey, err := cardano.LoadSigningKey(skeyPath)
	if err != nil {
		return err
	}

	if !isJSON {
		fmt.Fprintf(os.Stderr, "Signing transaction...\n")
	}

	// Sign
	result, err := cardano.SignTransaction(txHex, privKey, pubKey)
	if err != nil {
		return err
	}

	if isJSON {
		return output.PrintJSON(result)
	}

	if len(result.SignedTx) > 32 {
		fmt.Printf("Signed TX: %s...%s\n", result.SignedTx[:16], result.SignedTx[len(result.SignedTx)-16:])
	} else {
		fmt.Printf("Signed TX: %s\n", result.SignedTx)
	}
	fmt.Printf("TX Hash:   %s\n", result.TxHash)
	fmt.Fprintf(os.Stderr, "\nNext: andamio tx submit --tx <signed_tx> --submit-url <url>\n")
	return nil
}
