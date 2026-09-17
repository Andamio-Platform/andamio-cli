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

func TestResolveWalletAddress_FlagValueTakesPrecedence(t *testing.T) {
	// Points HOME somewhere with no wallet at all — if the flag value were
	// ignored and the fallback path taken instead, this would fail loudly.
	t.Setenv("HOME", t.TempDir())

	const flagValue = "addr_test1qexplicitlyprovidedbytheuser"
	got, err := ResolveWalletAddress(flagValue, "preprod")
	if err != nil {
		t.Fatalf("ResolveWalletAddress: %v", err)
	}
	if got != flagValue {
		t.Errorf("got %q, want flag value %q unchanged", got, flagValue)
	}
}

func TestResolveWalletAddress_FallsBackToAddressTxt(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	wallet, err := GenerateWallet("preprod")
	if err != nil {
		t.Fatalf("GenerateWallet: %v", err)
	}
	dir, err := DefaultWalletDir("default")
	if err != nil {
		t.Fatalf("DefaultWalletDir: %v", err)
	}
	if _, err := wallet.WriteFiles(dir, true, false); err != nil {
		t.Fatalf("WriteFiles: %v", err)
	}

	got, err := ResolveWalletAddress("", "preprod")
	if err != nil {
		t.Fatalf("ResolveWalletAddress: %v", err)
	}
	if got != wallet.PaymentAddress {
		t.Errorf("got %q, want %q (default wallet's address.txt)", got, wallet.PaymentAddress)
	}
}

// Covers a wallet created before address.txt existed: WriteFiles always
// writes it now, so this simulates the one on Andrew's machine that
// predates the change by deleting it after the fact.
func TestResolveWalletAddress_FallsBackToVKeysWhenAddressTxtMissing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	wallet, err := GenerateWallet("preprod")
	if err != nil {
		t.Fatalf("GenerateWallet: %v", err)
	}
	dir, err := DefaultWalletDir("default")
	if err != nil {
		t.Fatalf("DefaultWalletDir: %v", err)
	}
	if _, err := wallet.WriteFiles(dir, true, false); err != nil {
		t.Fatalf("WriteFiles: %v", err)
	}
	if err := os.Remove(filepath.Join(dir, "address.txt")); err != nil {
		t.Fatalf("failed to remove address.txt: %v", err)
	}

	got, err := ResolveWalletAddress("", "preprod")
	if err != nil {
		t.Fatalf("ResolveWalletAddress: %v", err)
	}
	if got != wallet.PaymentAddress {
		t.Errorf("got %q, want %q (derived from vkeys)", got, wallet.PaymentAddress)
	}
}

func TestResolveWalletAddress_NoDefaultWalletReturnsActionableError(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	_, err := ResolveWalletAddress("", "preprod")
	if err == nil {
		t.Fatal("expected error when no default wallet exists, got nil")
	}
	if !strings.Contains(err.Error(), "wallet create") {
		t.Errorf("error %q does not point the user at 'wallet create'", err.Error())
	}
}
