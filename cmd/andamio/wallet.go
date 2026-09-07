package main

import (
	"fmt"
	"os"

	"github.com/Andamio-Platform/andamio-cli/internal/cardano"
	"github.com/Andamio-Platform/andamio-cli/internal/output"
	"github.com/spf13/cobra"
)

var walletCmd = &cobra.Command{
	Use:   "wallet",
	Short: "Local Cardano wallet management",
}

var walletCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Generate a new local signing wallet (.skey/.vkey)",
	Long: `Generate a new Cardano payment/stake key pair entirely offline, for
use with 'tx sign --skey' and 'tx run --skey'.

Writes payment.skey, payment.vkey, stake.skey, stake.vkey (and, unless
--no-write-mnemonic is set, mnemonic.txt) into a wallet directory. By
default this is ~/.andamio/wallet/<name>/ (--name defaults to "default"),
so a fresh install has a usable signing key with zero flags — 'tx sign'
and 'tx run' fall back to this path when --skey is omitted. Use
--output-dir to write somewhere else instead.

The mnemonic is written to disk (0600) rather than only shown once: every
CLI command must work without a TTY, so there's no interactive "did you
save it?" gate this could wait on. Pass --no-write-mnemonic to opt back
into shown-once-only (e.g. for a mainnet wallet you'd rather not persist).

Examples:
  andamio wallet create
  andamio wallet create --name treasury --network mainnet
  andamio wallet create --output-dir ./payment --output json`,
	RunE: runWalletCreate,
}

func init() {
	rootCmd.AddCommand(walletCmd)
	walletCmd.AddCommand(walletCreateCmd)

	walletCreateCmd.Flags().
		String("name", "default", "Wallet name — stored under ~/.andamio/wallet/<name>/")
	walletCreateCmd.Flags().
		String("output-dir", "", "Write keys here instead of ~/.andamio/wallet/<name>/")
	walletCreateCmd.Flags().
		String("network", "preprod", "Network to derive the address for (preprod, mainnet, preview)")
	walletCreateCmd.Flags().
		Bool("no-write-mnemonic", false, "Do not write mnemonic.txt — print it once to stderr instead")
}

func runWalletCreate(cmd *cobra.Command, args []string) error {
	name, _ := cmd.Flags().GetString("name")
	outputDir, _ := cmd.Flags().GetString("output-dir")
	network, _ := cmd.Flags().GetString("network")
	noWriteMnemonic, _ := cmd.Flags().GetBool("no-write-mnemonic")
	isJSON := output.GetFormat() == output.FormatJSON

	dir := outputDir
	if dir == "" {
		var err error
		dir, err = cardano.DefaultWalletDir(name)
		if err != nil {
			return err
		}
	}

	if !isJSON {
		fmt.Fprintf(os.Stderr, "Generating wallet for network %q...\n", network)
	}

	wallet, err := cardano.GenerateWallet(network)
	if err != nil {
		return err
	}

	paths, err := wallet.WriteFiles(dir, noWriteMnemonic)
	if err != nil {
		return err
	}

	if noWriteMnemonic {
		fmt.Fprintf(os.Stderr,
			"\nMnemonic (shown once — write it down now, it will not be shown again):\n\n  %s\n\n",
			wallet.Mnemonic)
	} else if !isJSON {
		fmt.Fprintf(os.Stderr, "Wrote mnemonic to %s — back this up somewhere safe.\n", paths["mnemonic.txt"])
	}

	result := map[string]string{
		"address":           wallet.PaymentAddress,
		"stake_address":     wallet.StakeAddress,
		"payment_skey_path": paths["payment.skey"],
		"payment_vkey_path": paths["payment.vkey"],
		"stake_skey_path":   paths["stake.skey"],
		"stake_vkey_path":   paths["stake.vkey"],
	}
	if p, ok := paths["mnemonic.txt"]; ok {
		result["mnemonic_path"] = p
	}

	if isJSON {
		return output.PrintJSON(result)
	}

	fmt.Printf("Address:      %s\n", wallet.PaymentAddress)
	fmt.Printf("Stake addr:   %s\n", wallet.StakeAddress)
	fmt.Printf("Payment skey: %s\n", paths["payment.skey"])
	fmt.Fprintf(os.Stderr, "\nNext: andamio tx sign --tx <cbor> --skey %s\n", paths["payment.skey"])
	return nil
}
