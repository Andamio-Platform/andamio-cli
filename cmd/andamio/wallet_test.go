package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Andamio-Platform/andamio-cli/internal/cardano"
	"github.com/Andamio-Platform/andamio-cli/internal/output"
	"github.com/spf13/cobra"
)

// newWalletAddressTestCmd builds a fresh command per test so flag state
// doesn't leak between tests. RunE is the production runWalletAddress.
func newWalletAddressTestCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "address", RunE: runWalletAddress, Args: cobra.NoArgs}
	cmd.Flags().String("name", "default", "")
	cmd.Flags().String("dir", "", "")
	cmd.Flags().String("network", "preprod", "")
	return cmd
}

// writeTestWallet generates a preprod wallet into dir and returns it.
func writeTestWallet(t *testing.T, dir string) *cardano.GeneratedWallet {
	t.Helper()
	wallet, err := cardano.GenerateWallet("preprod")
	if err != nil {
		t.Fatalf("GenerateWallet: %v", err)
	}
	if _, err := wallet.WriteFiles(dir, true, false); err != nil {
		t.Fatalf("WriteFiles: %v", err)
	}
	return wallet
}

func TestRunWalletAddress_NamedWallet_TextPrintsBareAddress(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	wallet := writeTestWallet(t, filepath.Join(home, ".andamio", "wallet", "treasury"))

	cmd := newWalletAddressTestCmd()
	if err := cmd.Flags().Set("name", "treasury"); err != nil {
		t.Fatal(err)
	}
	var runErr error
	out := captureStdout(t, func() { runErr = cmd.RunE(cmd, []string{}) })
	if runErr != nil {
		t.Fatalf("runWalletAddress: %v", runErr)
	}

	if out != wallet.PaymentAddress+"\n" {
		t.Errorf("stdout = %q, want bare address %q", out, wallet.PaymentAddress+"\n")
	}
}

func TestRunWalletAddress_Dir_JSON(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	wallet := writeTestWallet(t, dir)

	_ = output.SetFormat("json")
	t.Cleanup(func() { _ = output.SetFormat("text") })

	cmd := newWalletAddressTestCmd()
	if err := cmd.Flags().Set("dir", dir); err != nil {
		t.Fatal(err)
	}
	var runErr error
	out := captureStdout(t, func() { runErr = cmd.RunE(cmd, []string{}) })
	if runErr != nil {
		t.Fatalf("runWalletAddress: %v", runErr)
	}

	var got map[string]interface{}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, out)
	}
	if got["address"] != wallet.PaymentAddress {
		t.Errorf("address = %v, want %q", got["address"], wallet.PaymentAddress)
	}
	if got["wallet_dir"] != dir {
		t.Errorf("wallet_dir = %v, want %q", got["wallet_dir"], dir)
	}
	if got["source"] != "address.txt" {
		t.Errorf("source = %v, want %q", got["source"], "address.txt")
	}
}

func TestRunWalletAddress_DerivesWhenAddressTxtMissing(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	wallet := writeTestWallet(t, dir)
	if err := os.Remove(filepath.Join(dir, "address.txt")); err != nil {
		t.Fatal(err)
	}

	_ = output.SetFormat("json")
	t.Cleanup(func() { _ = output.SetFormat("text") })

	cmd := newWalletAddressTestCmd()
	if err := cmd.Flags().Set("dir", dir); err != nil {
		t.Fatal(err)
	}
	var runErr error
	out := captureStdout(t, func() { runErr = cmd.RunE(cmd, []string{}) })
	if runErr != nil {
		t.Fatalf("runWalletAddress: %v", runErr)
	}

	var got map[string]interface{}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, out)
	}
	if got["address"] != wallet.PaymentAddress {
		t.Errorf("address = %v, want %q", got["address"], wallet.PaymentAddress)
	}
	if got["source"] != "vkeys" {
		t.Errorf("source = %v, want %q", got["source"], "vkeys")
	}
}

func TestRunWalletAddress_NameAndDirMutuallyExclusive(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cmd := newWalletAddressTestCmd()
	if err := cmd.Flags().Set("name", "treasury"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("dir", t.TempDir()); err != nil {
		t.Fatal(err)
	}

	err := cmd.RunE(cmd, []string{})
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("err = %v, want mutually-exclusive error", err)
	}
}

func TestRunWalletAddress_NoWallet(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cmd := newWalletAddressTestCmd()

	err := cmd.RunE(cmd, []string{})
	if err == nil || !strings.Contains(err.Error(), "wallet create") {
		t.Fatalf("err = %v, want error pointing at 'wallet create'", err)
	}
}
