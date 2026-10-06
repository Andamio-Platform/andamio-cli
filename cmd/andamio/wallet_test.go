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

// newWalletCreateTestCmd builds a fresh wallet create command per test so
// flag state (and Changed) doesn't leak between tests. RunE is the
// production runWalletCreate.
func newWalletCreateTestCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "create", RunE: runWalletCreate, Args: cobra.NoArgs}
	cmd.Flags().String("name", "default", "")
	cmd.Flags().String("output-dir", "", "")
	cmd.Flags().String("network", "preprod", "")
	cmd.Flags().Bool("no-write-mnemonic", false, "")
	cmd.Flags().Bool("force", false, "")
	return cmd
}

// setupWalletCreateHome points HOME at a temp dir with a config whose
// gateway is baseURL, and switches output to JSON for the test.
func setupWalletCreateHome(t *testing.T, baseURL string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfgDir := filepath.Join(home, ".andamio")
	if err := os.MkdirAll(cfgDir, 0700); err != nil {
		t.Fatal(err)
	}
	cfg, _ := json.Marshal(map[string]string{"base_url": baseURL})
	if err := os.WriteFile(filepath.Join(cfgDir, "config.json"), cfg, 0600); err != nil {
		t.Fatal(err)
	}
	_ = output.SetFormat("json")
	t.Cleanup(func() { _ = output.SetFormat("text") })
	return home
}

// runWalletCreateJSON runs cmd and decodes its JSON stdout.
func runWalletCreateJSON(t *testing.T, cmd *cobra.Command) (map[string]interface{}, error) {
	t.Helper()
	var runErr error
	out := captureStdout(t, func() {
		_ = captureStderr(t, func() { runErr = cmd.RunE(cmd, []string{}) })
	})
	if runErr != nil {
		return nil, runErr
	}
	var got map[string]interface{}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, out)
	}
	return got, nil
}

func TestRunWalletCreate_NoNetworkFollowsConfiguredGateway(t *testing.T) {
	setupWalletCreateHome(t, "https://mainnet.api.andamio.io")

	got, err := runWalletCreateJSON(t, newWalletCreateTestCmd())
	if err != nil {
		t.Fatalf("runWalletCreate: %v", err)
	}
	// The --network flag defaults to preprod, but with it omitted the
	// wallet must match the mainnet gateway instead.
	addr, _ := got["address"].(string)
	if !strings.HasPrefix(addr, "addr1") {
		t.Errorf("address = %q, want a mainnet addr1… address", addr)
	}
	if _, ok := got["warnings"]; ok {
		t.Errorf("unexpected warnings: %v", got["warnings"])
	}
}

func TestRunWalletCreate_NetworkMismatchWarnsInJSON(t *testing.T) {
	setupWalletCreateHome(t, "https://preprod.api.andamio.io")

	cmd := newWalletCreateTestCmd()
	if err := cmd.Flags().Set("network", "mainnet"); err != nil {
		t.Fatal(err)
	}
	got, err := runWalletCreateJSON(t, cmd)
	if err != nil {
		t.Fatalf("runWalletCreate: %v", err)
	}
	// Explicit --network wins over the gateway, but the caller is told.
	addr, _ := got["address"].(string)
	if !strings.HasPrefix(addr, "addr1") {
		t.Errorf("address = %q, want a mainnet addr1… address", addr)
	}
	warnings, _ := got["warnings"].([]interface{})
	if len(warnings) != 1 || !strings.Contains(warnings[0].(string), "does not match the configured gateway") {
		t.Errorf("warnings = %v, want one network-mismatch warning", got["warnings"])
	}
}

func TestRunWalletCreate_RerunNeedsForce(t *testing.T) {
	home := setupWalletCreateHome(t, "https://preprod.api.andamio.io")

	first, err := runWalletCreateJSON(t, newWalletCreateTestCmd())
	if err != nil {
		t.Fatalf("first runWalletCreate: %v", err)
	}
	firstAddr, _ := first["address"].(string)
	if !strings.HasPrefix(firstAddr, "addr_test1") {
		t.Fatalf("address = %q, want a preprod addr_test1… address", firstAddr)
	}

	// Without --force: refused, first wallet untouched.
	if _, err := runWalletCreateJSON(t, newWalletCreateTestCmd()); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("rerun without --force: err = %v, want 'already exists'", err)
	}
	addrFile := filepath.Join(home, ".andamio", "wallet", "default", "address.txt")
	if b, _ := os.ReadFile(addrFile); strings.TrimSpace(string(b)) != firstAddr {
		t.Errorf("refused rerun changed address.txt to %q, want %q", b, firstAddr)
	}

	// With --force: replaced by a new wallet.
	cmd := newWalletCreateTestCmd()
	if err := cmd.Flags().Set("force", "true"); err != nil {
		t.Fatal(err)
	}
	second, err := runWalletCreateJSON(t, cmd)
	if err != nil {
		t.Fatalf("rerun with --force: %v", err)
	}
	secondAddr, _ := second["address"].(string)
	if secondAddr == firstAddr {
		t.Error("--force rerun kept the old address, want a new wallet")
	}
	if b, _ := os.ReadFile(addrFile); strings.TrimSpace(string(b)) != secondAddr {
		t.Errorf("address.txt = %q after --force, want %q", b, secondAddr)
	}
}
