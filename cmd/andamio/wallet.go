package main

import (
	"fmt"
	"os"

	"github.com/Andamio-Platform/andamio-cli/internal/cardano"
	"github.com/Andamio-Platform/andamio-cli/internal/config"
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

--network picks the Cardano address prefix (preprod: addr_test1..., mainnet:
addr1...) and defaults to whatever the configured gateway (BaseURL, see
'andamio config set-url') looks like rather than always "preprod" — a
mainnet-configured CLI gets a mainnet wallet with zero flags too. Pass
--network explicitly to override; if it disagrees with the configured
gateway, a warning is printed but the wallet is still generated as asked.

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
		String("network", "preprod", "Network to derive the address for (preprod, mainnet, preview) — defaults to the configured gateway's network when not given")
	walletCreateCmd.Flags().
		Bool("no-write-mnemonic", false, "Do not write mnemonic.txt — print it once to stderr instead")
}

func runWalletCreate(cmd *cobra.Command, args []string) error {
	name, _ := cmd.Flags().GetString("name")
	outputDir, _ := cmd.Flags().GetString("output-dir")
	network, _ := cmd.Flags().GetString("network")
	networkExplicit := cmd.Flags().Changed("network")
	noWriteMnemonic, _ := cmd.Flags().GetBool("no-write-mnemonic")
	isJSON := output.GetFormat() == output.FormatJSON

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	configuredNetwork := config.NetworkForBaseURL(cfg.BaseURL)

	switch {
	case !networkExplicit && configuredNetwork != "":
		// No --network given: match the gateway the CLI is actually
		// configured against instead of always defaulting to preprod.
		network = configuredNetwork
	case networkExplicit && configuredNetwork != "" && network != configuredNetwork:
		fmt.Fprintf(os.Stderr,
			"Warning: --network %s does not match the configured gateway (%s, which looks like %s). Generating a %s wallet anyway, as requested — run 'andamio config set-url' if that gateway is wrong, or drop --network to match it.\n",
			network, cfg.BaseURL, configuredNetwork, network)
	}

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

	printSecurityWarnings(wallet, dir, network, noWriteMnemonic, paths)

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

// printSecurityWarnings echoes key/mnemonic handling guidance to stderr,
// scaled to what this run actually did (mnemonic written vs. shown-once,
// preprod/preview vs. mainnet). Always printed regardless of --output —
// stderr doesn't touch the JSON contract on stdout, and scripted callers
// are exactly the audience most likely to never see it otherwise.
func printSecurityWarnings(wallet *cardano.GeneratedWallet, dir, network string, noWriteMnemonic bool, paths map[string]string) {
	fmt.Fprintln(os.Stderr)
	fmt.Fprintf(os.Stderr, "SECURITY WARNING: %s (and %s) can sign transactions and move every fund at this address — with no further confirmation from you.\n", paths["payment.skey"], paths["stake.skey"])
	fmt.Fprintf(os.Stderr, "  - Keep %s private. Never commit it to git, sync it to cloud storage/shared drives, or paste its contents into chat, an issue, a log, or an AI tool.\n", dir)
	fmt.Fprintln(os.Stderr, "  - The files on disk are 0600, but any copy you make (backup drive, password manager entry) only keeps that protection if you set it yourself — check permissions on copies too.")
	fmt.Fprintln(os.Stderr, "  - If this key is ever exposed, funds can be moved instantly and irreversibly. There is no support desk, chargeback, or recovery — treat exposure as a total loss of everything at this address.")

	if noWriteMnemonic {
		fmt.Fprintf(os.Stderr,
			"\nMnemonic (shown once — not written to disk by this command, will not be shown again):\n\n  %s\n\n",
			wallet.Mnemonic)
		fmt.Fprintln(os.Stderr, "Write it down now, offline, before doing anything else. If it's lost, this wallet is unrecoverable — there is no reset and no account recovery.")
	} else {
		fmt.Fprintf(os.Stderr, "  - %s is written in plaintext and is even more sensitive than the .skey files: it can regenerate every key derived from it, including any future accounts on this wallet.\n", paths["mnemonic.txt"])
		fmt.Fprintln(os.Stderr, "    Move it to a password manager or encrypted storage and delete the plaintext copy once it's backed up, or rerun with --no-write-mnemonic to avoid writing it to disk at all.")
	}

	if network == "mainnet" {
		fmt.Fprintln(os.Stderr, "  - This is a MAINNET wallet: real ADA, not test funds. For anything beyond small amounts, consider a hardware wallet instead of a CLI-generated hot key.")
	}
	fmt.Fprintln(os.Stderr)
}
