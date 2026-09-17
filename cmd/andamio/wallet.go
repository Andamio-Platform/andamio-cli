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

Writes payment.skey, payment.vkey, stake.skey, stake.vkey, address.txt
(and, unless --no-write-mnemonic is set, mnemonic.txt) into a wallet
directory. By
default this is ~/.andamio/wallet/<name>/ (--name defaults to "default"),
so a fresh install has a usable signing key with zero flags — 'tx sign'
and 'tx run' fall back to this path when --skey is omitted. Use
--output-dir to write somewhere else instead.

--network picks the Cardano address prefix (preprod: addr_test1..., mainnet:
addr1...) and defaults to whatever the configured gateway (BaseURL, see
'andamio config set-url') looks like rather than always "preprod" — a
mainnet-configured CLI gets a mainnet wallet with zero flags too. Pass
--network explicitly to override; if it disagrees with the configured
gateway, the wallet is still generated as asked, but a warning is printed
to stderr AND included as a "warnings" array in --output json — scripted
and agent callers that only check the JSON result still see it.

Refuses to run if a wallet already exists at the target directory — there
is no interactive overwrite prompt anywhere in this CLI, so silently
clobbering an existing (possibly funded) wallet on a re-run or retry is
not an acceptable default. Pass --force to overwrite anyway.

The mnemonic is written to disk (0600) rather than only shown once: every
CLI command must work without a TTY, so there's no interactive "did you
save it?" gate this could wait on. Pass --no-write-mnemonic to opt back
into shown-once-only (e.g. for a mainnet wallet you'd rather not persist).

WHAT THIS WALLET CAN'T DO YET

This is a bare keypair: no funds, no Access Token, and on its own it
cannot authenticate anywhere. Two more things, neither of them wallet
files, are required before it's useful for 'user login' / 'dev login':

  Access Token   An on-chain NFT carrying a chosen alias. Both logins work
                 by signing a nonce with this wallet's key AND the gateway
                 confirming the wallet holds an Access Token for the alias
                 claimed — a valid signature from an empty wallet is not
                 enough. This command mints no token. Get one onto this
                 wallet with:
                   andamio tx build /v2/tx/global/user/access-token/mint \
                     --body '{"alias":"<name>","initiator_data":{"change_address":"<addr>","used_addresses":["<addr>"]}}'
                 which needs an *already-authenticated* session to call
                 (not this new wallet — see 'andamio user login') and
                 needs this wallet funded first (test ADA on preprod, real
                 ADA on mainnet — see README.md#networks).

  API key        Not minted by this command and not obtainable via the
                 CLI at all. Get one by connecting the wallet that holds
                 your Access Token at:
                   preprod: https://preprod.app.andamio.io/api-setup
                   mainnet: https://app.andamio.io/api-setup
                 then store it locally with:
                   andamio auth login --api-key <key>
                 Only one key is stored at a time: switching 'andamio
                 config set-url' between preprod and mainnet does NOT
                 keep the previous network's key around, and there is no
                 local backup of it once overwritten — save both keys
                 somewhere yourself (password manager, etc.) before
                 switching, or you'll need to re-visit api-setup to
                 recover the one you left behind.

Until both are in place, this wallet is only good for 'tx sign' / 'tx run'
on transactions someone else's already-authenticated session built for it.

Examples:
  andamio wallet create
  andamio wallet create --name treasury --network mainnet
  andamio wallet create --output-dir ./payment --output json
  andamio wallet create --force`,
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
	walletCreateCmd.Flags().
		Bool("force", false, "Overwrite an existing wallet at the target directory")
}

func runWalletCreate(cmd *cobra.Command, args []string) error {
	name, _ := cmd.Flags().GetString("name")
	outputDir, _ := cmd.Flags().GetString("output-dir")
	network, _ := cmd.Flags().GetString("network")
	networkExplicit := cmd.Flags().Changed("network")
	noWriteMnemonic, _ := cmd.Flags().GetBool("no-write-mnemonic")
	force, _ := cmd.Flags().GetBool("force")
	isJSON := output.GetFormat() == output.FormatJSON

	var warnings []string

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
		msg := fmt.Sprintf(
			"--network %s does not match the configured gateway (%s, which looks like %s). Generating a %s wallet anyway, as requested — run 'andamio config set-url' if that gateway is wrong, or drop --network to match it.",
			network, cfg.BaseURL, configuredNetwork, network)
		fmt.Fprintf(os.Stderr, "Warning: %s\n", msg)
		warnings = append(warnings, msg)
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

	paths, err := wallet.WriteFiles(dir, noWriteMnemonic, force)
	if err != nil {
		return err
	}

	printSecurityWarnings(wallet, dir, network, noWriteMnemonic, paths)

	result := map[string]interface{}{
		"address":           wallet.PaymentAddress,
		"address_path":      paths["address.txt"],
		"stake_address":     wallet.StakeAddress,
		"payment_skey_path": paths["payment.skey"],
		"payment_vkey_path": paths["payment.vkey"],
		"stake_skey_path":   paths["stake.skey"],
		"stake_vkey_path":   paths["stake.vkey"],
	}
	if p, ok := paths["mnemonic.txt"]; ok {
		result["mnemonic_path"] = p
	}
	if len(warnings) > 0 {
		result["warnings"] = warnings
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
