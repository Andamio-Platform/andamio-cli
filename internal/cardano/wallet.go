package cardano

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

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

	return &GeneratedWallet{
		Mnemonic:       wallet.Mnemonic,
		PaymentAddress: wallet.PaymentAddress,
		StakeAddress:   wallet.StakeAddress,
		PaymentVKey:    keyFiles["payment.vkey"],
		PaymentSKey:    keyFiles["payment.skey"],
		StakeVKey:      keyFiles["stake.vkey"],
		StakeSKey:      keyFiles["stake.skey"],
	}, nil
}

// WriteFiles writes the wallet's payment/stake key files into dir (created
// if necessary) as 0600 files — the same cardano-cli JSON envelope format
// LoadSigningKey already reads. The mnemonic is written too as
// mnemonic.txt, unless skipMnemonic is set, in which case the caller is
// responsible for showing it to the user some other way (it is not
// recoverable afterwards). Returns the absolute path written for each file,
// keyed by filename.
func (w *GeneratedWallet) WriteFiles(dir string, skipMnemonic bool) (map[string]string, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create wallet directory: %w", err)
	}

	files := map[string]string{
		"payment.vkey": w.PaymentVKey,
		"payment.skey": w.PaymentSKey,
		"stake.vkey":   w.StakeVKey,
		"stake.skey":   w.StakeSKey,
	}
	if !skipMnemonic {
		files["mnemonic.txt"] = w.Mnemonic + "\n"
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
