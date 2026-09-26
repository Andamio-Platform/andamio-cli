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
