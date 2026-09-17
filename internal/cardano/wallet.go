package cardano

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	ouroboros "github.com/blinklabs-io/gouroboros"
	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"

	"github.com/blinklabs-io/bursa"
)

// GeneratedWallet holds a freshly derived payment/stake key pair, ready to
// be written to disk. It intentionally exposes only the payment and stake
// material — Bursa's Wallet also derives DRep/committee/pool-cold keys,
// which andamio-cli's tx-signing surface has no use for.
type GeneratedWallet struct {
	Mnemonic       string
	PaymentAddress string
	StakeAddress   string
	PaymentVKey    string
	PaymentSKey    string
	StakeVKey      string
	StakeSKey      string
}

// GenerateWallet creates a fresh BIP-39 mnemonic and derives a CIP-1852
// payment/stake key set for the given network ("preprod", "mainnet",
// "preview", ...) via Bursa, entirely offline.
func GenerateWallet(network string) (*GeneratedWallet, error) {
	mnemonic, err := bursa.GenerateMnemonic()
	if err != nil {
		return nil, fmt.Errorf("failed to generate mnemonic: %w", err)
	}

	wallet, err := bursa.NewWallet(mnemonic, bursa.WithNetwork(network))
	if err != nil {
		return nil, fmt.Errorf("failed to derive wallet keys: %w", err)
	}

	keyFiles, err := bursa.ExtractKeyFiles(wallet)
	if err != nil {
		return nil, fmt.Errorf("failed to encode key files: %w", err)
	}

	// Deliberately NOT keyFiles["payment.skey"]/["stake.skey"]: Bursa writes
	// those under the type "PaymentSigningKeyShelley_ed25519" — the label
	// for a genuinely random 32-byte Ed25519 seed — but the bytes it puts
	// there are actually just kL, the first half of the BIP32-derived
	// (kL, kR) pair (see Bursa's getSigningKeyFile: "Use first 32 bytes
	// (k_L) of the 64-byte extended private key"). kR is silently dropped.
	// Any correctly-behaving loader (including Bursa's own
	// LoadKeyFromFile) sees the "PaymentSigningKeyShelley_ed25519" label
	// and treats those 32 bytes as a seed to re-hash via RFC 8032 — which
	// for a real seed is right, but for a bare kL produces a completely
	// different, wrong keypair than the one that actually owns this
	// wallet's address. There is no way to sign correctly from the
	// mislabeled file at all: kR isn't in it.
	//
	// keyFiles["paymentExtended.skey"]/["stakeExtended.skey"] are Bursa's
	// OTHER output for the same derived key: the full 128-byte cardano-cli
	// extended format (kL||kR||pubkey||chaincode) under the correct
	// "...ExtendedSigningKeyShelley_ed25519_bip32" label, which
	// LoadSigningKey's extended-key path (internal/cardano/sign.go) signs
	// correctly. Written to disk as payment.skey/stake.skey regardless —
	// the .skey filename and cardano-cli JSON envelope shape are unchanged,
	// only which of Bursa's two outputs backs them.
	return &GeneratedWallet{
		Mnemonic:       wallet.Mnemonic,
		PaymentAddress: wallet.PaymentAddress,
		StakeAddress:   wallet.StakeAddress,
		PaymentVKey:    keyFiles["payment.vkey"],
		PaymentSKey:    keyFiles["paymentExtended.skey"],
		StakeVKey:      keyFiles["stake.vkey"],
		StakeSKey:      keyFiles["stakeExtended.skey"],
	}, nil
}

// WriteFiles writes the wallet's payment/stake key files into dir (created
// if necessary) as 0600 files — the same cardano-cli JSON envelope format
// LoadSigningKey already reads. The mnemonic is written too as
// mnemonic.txt, unless skipMnemonic is set, in which case the caller is
// responsible for showing it to the user some other way (it is not
// recoverable afterwards). Returns the absolute path written for each file,
// keyed by filename.
//
// Unless force is set, WriteFiles refuses to run if any target file already
// exists, and writes nothing. This isn't just a courtesy prompt substitute —
// there is no interactive "did you mean to overwrite this?" gate anywhere in
// the CLI (see the no-prompts convention), so an unconditional overwrite is
// one retried or re-run command away from silently destroying an existing,
// possibly-funded wallet with no recovery path unless its mnemonic was
// separately backed up. The pre-check runs before any file is written, so a
// rejected call never leaves a directory half-overwritten.
func (w *GeneratedWallet) WriteFiles(dir string, skipMnemonic bool, force bool) (map[string]string, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create wallet directory: %w", err)
	}

	files := map[string]string{
		"payment.vkey": w.PaymentVKey,
		"payment.skey": w.PaymentSKey,
		"stake.vkey":   w.StakeVKey,
		"stake.skey":   w.StakeSKey,
		// Public info (bursa derives it from the account xprv at wallet
		// creation and hands it back as PaymentAddress) — persisted here so
		// ResolveWalletAddress can just read it back rather than
		// re-deriving it from the vkeys later. Previously wallet create
		// only ever printed this once and threw it away.
		"address.txt": w.PaymentAddress + "\n",
	}
	if !skipMnemonic {
		files["mnemonic.txt"] = w.Mnemonic + "\n"
	}

	if !force {
		for name := range files {
			path := filepath.Join(dir, name)
			if _, err := os.Stat(path); err == nil {
				return nil, fmt.Errorf("wallet already exists at %s (found %s) — pass --force to overwrite it, or --name/--output-dir to write a new one elsewhere", dir, path)
			} else if !errors.Is(err, os.ErrNotExist) {
				return nil, fmt.Errorf("failed to check %s: %w", path, err)
			}
		}
	}

	paths := make(map[string]string, len(files))
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			return nil, fmt.Errorf("failed to write %s: %w", name, err)
		}
		paths[name] = path
	}
	return paths, nil
}

// DefaultWalletDir returns ~/.andamio/wallet/<name> — the CLI's one
// filesystem convention (alongside ~/.andamio/config.json) for a
// locally-generated wallet.
func DefaultWalletDir(name string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to resolve home directory: %w", err)
	}
	return filepath.Join(home, ".andamio", "wallet", name), nil
}

// ResolveSkeyPath returns flagValue unchanged if set. Otherwise it falls
// back to the default wallet's payment.skey (~/.andamio/wallet/default/),
// returning an error that tells the user how to get one if it doesn't
// exist — never prompts, never reads stdin.
func ResolveSkeyPath(flagValue string) (string, error) {
	if flagValue != "" {
		return flagValue, nil
	}

	dir, err := DefaultWalletDir("default")
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, "payment.skey")
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("--skey not given and no default wallet found at %s — run 'andamio wallet create' first, or pass --skey explicitly", path)
		}
		return "", fmt.Errorf("failed to check default wallet: %w", err)
	}
	return path, nil
}

// AddressFromVKeys derives a bech32 Shelley base address (payment + stake
// credential) from a payment.vkey/stake.vkey pair on disk. It reads only
// the public verification keys — never a .skey or mnemonic — since a base
// address is fully determined by the payment and stake key *hashes* alone.
// This is the same construction bursa.GetAddress uses internally, without
// needing the extended private key bursa.GetAddress requires: wallet
// create never persists an existing wallet's address anywhere (see
// clients/projects/andamio/NOTES.md, "Known UX Gaps" #5), so this is the
// fallback for recovering it from what *is* on disk.
func AddressFromVKeys(paymentVKeyPath, stakeVKeyPath, network string) (string, error) {
	paymentVKey, err := bursa.LoadKeyFromFile(paymentVKeyPath)
	if err != nil {
		return "", fmt.Errorf("failed to load %s: %w", paymentVKeyPath, err)
	}
	stakeVKey, err := bursa.LoadKeyFromFile(stakeVKeyPath)
	if err != nil {
		return "", fmt.Errorf("failed to load %s: %w", stakeVKeyPath, err)
	}

	net, ok := ouroboros.NetworkByName(network)
	if !ok {
		return "", fmt.Errorf("invalid network %q", network)
	}

	addr, err := lcommon.NewAddressFromParts(
		lcommon.AddressTypeKeyKey,
		net.Id,
		blake2b224(paymentVKey.VKey),
		blake2b224(stakeVKey.VKey),
	)
	if err != nil {
		return "", fmt.Errorf("failed to build address: %w", err)
	}
	return addr.String(), nil
}

// ResolveWalletAddress returns flagValue unchanged if set. Otherwise it
// falls back to the default wallet (~/.andamio/wallet/default/):
// address.txt if present (written by WriteFiles at creation time), or
// derived from payment.vkey/stake.vkey via AddressFromVKeys if not — the
// vkey path exists only to cover a wallet created before address.txt was
// introduced, since re-deriving is strictly worse than reading the value
// bursa already computed (extra decode work, and a second place the
// network/address-type logic could drift from bursa's own). Returns an
// error that tells the user how to get a wallet if none of the above
// exist — never prompts, never reads stdin. Mirrors ResolveSkeyPath.
func ResolveWalletAddress(flagValue, network string) (string, error) {
	if flagValue != "" {
		return flagValue, nil
	}

	dir, err := DefaultWalletDir("default")
	if err != nil {
		return "", err
	}

	addressPath := filepath.Join(dir, "address.txt")
	if data, err := os.ReadFile(addressPath); err == nil {
		return strings.TrimSpace(string(data)), nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("failed to read %s: %w", addressPath, err)
	}

	paymentVKeyPath := filepath.Join(dir, "payment.vkey")
	stakeVKeyPath := filepath.Join(dir, "stake.vkey")
	if _, err := os.Stat(paymentVKeyPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("--address not given and no default wallet found at %s — run 'andamio wallet create' first, or pass --address explicitly", dir)
		}
		return "", fmt.Errorf("failed to check default wallet: %w", err)
	}

	return AddressFromVKeys(paymentVKeyPath, stakeVKeyPath, network)
}
