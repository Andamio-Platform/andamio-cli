package cardano

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAddressFromVKeys_MatchesGeneratedWalletAddress(t *testing.T) {
	wallet, err := GenerateWallet("preprod")
	if err != nil {
		t.Fatalf("GenerateWallet: %v", err)
	}

	dir := t.TempDir()
	if _, err := wallet.WriteFiles(dir, true, false); err != nil {
		t.Fatalf("WriteFiles: %v", err)
	}

	got, err := AddressFromVKeys(
		filepath.Join(dir, "payment.vkey"),
		filepath.Join(dir, "stake.vkey"),
		"preprod",
	)
	if err != nil {
		t.Fatalf("AddressFromVKeys: %v", err)
	}

	if got != wallet.PaymentAddress {
		t.Errorf("address derived from vkeys = %q, want %q (wallet.PaymentAddress)", got, wallet.PaymentAddress)
	}
}

func TestAddressFromVKeys_InvalidNetwork(t *testing.T) {
	wallet, err := GenerateWallet("preprod")
	if err != nil {
		t.Fatalf("GenerateWallet: %v", err)
	}
	dir := t.TempDir()
	if _, err := wallet.WriteFiles(dir, true, false); err != nil {
		t.Fatalf("WriteFiles: %v", err)
	}

	_, err = AddressFromVKeys(
		filepath.Join(dir, "payment.vkey"),
		filepath.Join(dir, "stake.vkey"),
		"not-a-real-network",
	)
	if err == nil {
		t.Fatal("expected error for invalid network, got nil")
	}
}

func TestWalletAddress_ReadsAddressTxt(t *testing.T) {
	wallet, err := GenerateWallet("preprod")
	if err != nil {
		t.Fatalf("GenerateWallet: %v", err)
	}
	dir := t.TempDir()
	if _, err := wallet.WriteFiles(dir, true, false); err != nil {
		t.Fatalf("WriteFiles: %v", err)
	}

	// An invalid network proves address.txt was read: the derivation path
	// would reject it.
	got, derived, err := WalletAddress(dir, "not-a-network")
	if err != nil {
		t.Fatalf("WalletAddress: %v", err)
	}
	if derived {
		t.Error("derived = true, want false (address.txt present)")
	}
	if got != wallet.PaymentAddress {
		t.Errorf("got %q, want %q (address.txt)", got, wallet.PaymentAddress)
	}
}

// Covers a wallet created before address.txt existed: WriteFiles always
// writes it now, so this simulates an older wallet by deleting it.
func TestWalletAddress_DerivesFromVKeysWhenAddressTxtMissing(t *testing.T) {
	wallet, err := GenerateWallet("preprod")
	if err != nil {
		t.Fatalf("GenerateWallet: %v", err)
	}
	dir := t.TempDir()
	if _, err := wallet.WriteFiles(dir, true, false); err != nil {
		t.Fatalf("WriteFiles: %v", err)
	}
	if err := os.Remove(filepath.Join(dir, "address.txt")); err != nil {
		t.Fatalf("failed to remove address.txt: %v", err)
	}

	got, derived, err := WalletAddress(dir, "preprod")
	if err != nil {
		t.Fatalf("WalletAddress: %v", err)
	}
	if !derived {
		t.Error("derived = false, want true (address.txt missing)")
	}
	if got != wallet.PaymentAddress {
		t.Errorf("got %q, want %q (derived from vkeys)", got, wallet.PaymentAddress)
	}
}

func TestWalletAddress_NoWalletReturnsActionableError(t *testing.T) {
	_, _, err := WalletAddress(t.TempDir(), "preprod")
	if err == nil {
		t.Fatal("expected error when no wallet exists, got nil")
	}
	if !strings.Contains(err.Error(), "wallet create") {
		t.Errorf("error %q does not point the user at 'wallet create'", err.Error())
	}
}

func TestWriteFiles_RefusesExistingWalletWithoutForce(t *testing.T) {
	dir := t.TempDir()
	first, err := GenerateWallet("preprod")
	if err != nil {
		t.Fatalf("GenerateWallet: %v", err)
	}
	if _, err := first.WriteFiles(dir, false, false); err != nil {
		t.Fatalf("WriteFiles: %v", err)
	}

	second, err := GenerateWallet("preprod")
	if err != nil {
		t.Fatalf("GenerateWallet: %v", err)
	}
	if _, err := second.WriteFiles(dir, false, false); err == nil {
		t.Fatal("WriteFiles over an existing wallet without force: want error, got nil")
	}

	got, err := os.ReadFile(filepath.Join(dir, "address.txt"))
	if err != nil {
		t.Fatalf("read address.txt: %v", err)
	}
	if strings.TrimSpace(string(got)) != first.PaymentAddress {
		t.Errorf("rejected WriteFiles modified the existing wallet: address.txt = %q, want %q", got, first.PaymentAddress)
	}
}

// A directory holding only a mnemonic.txt is still an existing wallet, even
// when this run wouldn't write a mnemonic itself.
func TestWriteFiles_RefusesLoneMnemonicWithoutForce(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "mnemonic.txt"), []byte("old words\n"), 0600); err != nil {
		t.Fatalf("seed mnemonic.txt: %v", err)
	}

	wallet, err := GenerateWallet("preprod")
	if err != nil {
		t.Fatalf("GenerateWallet: %v", err)
	}
	if _, err := wallet.WriteFiles(dir, true, false); err == nil {
		t.Fatal("WriteFiles next to an existing mnemonic.txt without force: want error, got nil")
	}
	if _, err := os.Stat(filepath.Join(dir, "payment.skey")); !os.IsNotExist(err) {
		t.Errorf("rejected WriteFiles wrote payment.skey anyway (stat err = %v)", err)
	}
}

// --force --no-write-mnemonic over an existing wallet must not leave the old
// wallet's mnemonic next to the new keys.
func TestWriteFiles_ForceWithSkipMnemonicRemovesStaleMnemonic(t *testing.T) {
	dir := t.TempDir()
	first, err := GenerateWallet("preprod")
	if err != nil {
		t.Fatalf("GenerateWallet: %v", err)
	}
	if _, err := first.WriteFiles(dir, false, false); err != nil {
		t.Fatalf("WriteFiles: %v", err)
	}

	second, err := GenerateWallet("preprod")
	if err != nil {
		t.Fatalf("GenerateWallet: %v", err)
	}
	paths, err := second.WriteFiles(dir, true, true)
	if err != nil {
		t.Fatalf("WriteFiles with force: %v", err)
	}

	if _, ok := paths["mnemonic.txt"]; ok {
		t.Error("paths includes mnemonic.txt despite skipMnemonic")
	}
	if _, err := os.Stat(filepath.Join(dir, "mnemonic.txt")); !os.IsNotExist(err) {
		t.Errorf("previous wallet's mnemonic.txt still present after forced overwrite (stat err = %v)", err)
	}
}

// A --force overwrite that fails partway must not cost the existing wallet
// its mnemonic or leave temp files behind. A non-empty directory where
// address.txt should go makes that one file fail to land.
func TestWriteFiles_FailedForceKeepsMnemonicAndCleansUp(t *testing.T) {
	dir := t.TempDir()
	first, err := GenerateWallet("preprod")
	if err != nil {
		t.Fatalf("GenerateWallet: %v", err)
	}
	if _, err := first.WriteFiles(dir, false, false); err != nil {
		t.Fatalf("WriteFiles: %v", err)
	}

	blocker := filepath.Join(dir, "address.txt")
	if err := os.Remove(blocker); err != nil {
		t.Fatalf("remove address.txt: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(blocker, "keep"), 0700); err != nil {
		t.Fatalf("create blocking directory: %v", err)
	}

	second, err := GenerateWallet("preprod")
	if err != nil {
		t.Fatalf("GenerateWallet: %v", err)
	}
	if _, err := second.WriteFiles(dir, true, true); err == nil {
		t.Fatal("WriteFiles with address.txt blocked: want error, got nil")
	}

	got, err := os.ReadFile(filepath.Join(dir, "mnemonic.txt"))
	if err != nil {
		t.Fatalf("previous wallet's mnemonic.txt gone after failed overwrite: %v", err)
	}
	if strings.TrimSpace(string(got)) != first.Mnemonic {
		t.Errorf("mnemonic.txt changed after failed overwrite")
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp.") {
			t.Errorf("temp file left behind: %s", e.Name())
		}
	}
}

// A --force overwrite must leave every file at 0600, whatever the previous
// wallet's files were set to: os.WriteFile only applies its mode when it
// creates a file, so overwriting in place used to keep 0644 keys at 0644.
func TestWriteFiles_ForceResetsPermissionsTo0600(t *testing.T) {
	dir := t.TempDir()
	first, err := GenerateWallet("preprod")
	if err != nil {
		t.Fatalf("GenerateWallet: %v", err)
	}
	if _, err := first.WriteFiles(dir, false, false); err != nil {
		t.Fatalf("WriteFiles: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	for _, e := range entries {
		mode := os.FileMode(0644)
		if e.Name() == "payment.skey" {
			mode = 0400
		}
		if err := os.Chmod(filepath.Join(dir, e.Name()), mode); err != nil {
			t.Fatalf("chmod %s: %v", e.Name(), err)
		}
	}

	second, err := GenerateWallet("preprod")
	if err != nil {
		t.Fatalf("GenerateWallet: %v", err)
	}
	paths, err := second.WriteFiles(dir, false, true)
	if err != nil {
		t.Fatalf("WriteFiles with force over 0644/0400 files: %v", err)
	}

	for name, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", name, err)
		}
		if got := info.Mode().Perm(); got != 0600 {
			t.Errorf("%s mode = %o, want 600", name, got)
		}
	}

	got, err := os.ReadFile(filepath.Join(dir, "address.txt"))
	if err != nil {
		t.Fatalf("read address.txt: %v", err)
	}
	if strings.TrimSpace(string(got)) != second.PaymentAddress {
		t.Errorf("address.txt = %q, want the new wallet's %q", got, second.PaymentAddress)
	}
}

func TestResolveSkeyPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	defaultSkey := filepath.Join(home, ".andamio", "wallet", "default", "payment.skey")

	writeDefault := func(t *testing.T) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(defaultSkey), 0700); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(defaultSkey, []byte("{}"), 0600); err != nil {
			t.Fatalf("write default skey: %v", err)
		}
	}

	tests := []struct {
		name        string
		flagValue   string
		explicit    bool
		haveDefault bool
		wantPath    string
		wantDefault bool
		wantErr     string
	}{
		{name: "explicit path is used as given", flagValue: "/keys/mine.skey", explicit: true, haveDefault: true, wantPath: "/keys/mine.skey"},
		// An unset shell variable (--skey "$KEY") must not silently fall
		// back to the default wallet.
		{name: "explicit empty value errors", flagValue: "", explicit: true, haveDefault: true, wantErr: "--skey was given an empty value"},
		{name: "omitted falls back to default wallet", explicit: false, haveDefault: true, wantPath: defaultSkey, wantDefault: true},
		{name: "omitted with no default wallet errors", explicit: false, haveDefault: false, wantErr: "no default wallet found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = os.RemoveAll(filepath.Join(home, ".andamio"))
			if tt.haveDefault {
				writeDefault(t)
			}

			path, usedDefault, err := ResolveSkeyPath(tt.flagValue, tt.explicit)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want it to contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if path != tt.wantPath {
				t.Errorf("path = %q, want %q", path, tt.wantPath)
			}
			if usedDefault != tt.wantDefault {
				t.Errorf("usedDefault = %v, want %v", usedDefault, tt.wantDefault)
			}
		})
	}
}
