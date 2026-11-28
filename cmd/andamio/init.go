package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/Andamio-Platform/andamio-cli/internal/cardano"
	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/Andamio-Platform/andamio-cli/internal/config"
	"github.com/blinklabs-io/bursa"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Set up a new Andamio course and project from scratch",
	Long: `Interactive setup wizard for new Andamio developers.

Guides you through:
  1. Generating a Cardano keypair (no cardano-cli needed)
  2. Funding your preprod wallet via the testnet faucet
  3. Configuring your Blockfrost submit endpoint
  4. Authenticating headlessly with your new key
  5. Creating a course and project on-chain

Prerequisites:
  - andamio-cli installed
  - A free Blockfrost account (https://blockfrost.io) with a preprod project

Example:
  andamio init
  andamio init --keys-dir ./my-keys --course-title "My Course" --project-title "My Project"`,
	RunE: runInit,
}

func init() {
	rootCmd.AddCommand(initCmd)
	initCmd.Flags().String("keys-dir", ".", "Directory to write generated keypair files")
	initCmd.Flags().String("alias", "", "Andamio alias to use (skips prompt)")
	initCmd.Flags().String("blockfrost-id", "", "Blockfrost preprod project ID (skips prompt)")
	initCmd.Flags().String("course-title", "My First Course", "Title for the seed course")
	initCmd.Flags().String("project-title", "My First Project", "Title for the seed project")
	initCmd.Flags().Bool("skip-fund-check", false, "Skip the faucet funding pause (useful if wallet is already funded)")
}

func runInit(cmd *cobra.Command, args []string) error {
	keysDir, _ := cmd.Flags().GetString("keys-dir")
	aliasFlag, _ := cmd.Flags().GetString("alias")
	blockfrostFlag, _ := cmd.Flags().GetString("blockfrost-id")
	courseTitle, _ := cmd.Flags().GetString("course-title")
	projectTitle, _ := cmd.Flags().GetString("project-title")
	skipFundCheck, _ := cmd.Flags().GetBool("skip-fund-check")

	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "┌─────────────────────────────────────────┐")
	fmt.Fprintln(os.Stderr, "│         Andamio Init Wizard             │")
	fmt.Fprintln(os.Stderr, "└─────────────────────────────────────────┘")
	fmt.Fprintln(os.Stderr, "")

	// ── Step 1: Generate keypair ──────────────────────────────────────────
	fmt.Fprintln(os.Stderr, "Step 1/5  Generating Cardano keypair...")

	skeyPath, address, err := generateAndSaveKeypair(keysDir)
	if err != nil {
		return fmt.Errorf("failed to generate keypair: %w", err)
	}

	fmt.Fprintf(os.Stderr, "  ✓ payment.skey  → %s/payment.skey\n", keysDir)
	fmt.Fprintf(os.Stderr, "  ✓ payment.vkey  → %s/payment.vkey\n", keysDir)
	fmt.Fprintf(os.Stderr, "  ✓ payment.addr  → %s/payment.addr\n", keysDir)
	fmt.Fprintf(os.Stderr, "  Address: %s\n", address)
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "  Keep payment.skey safe — it controls your on-chain assets.")
	fmt.Fprintln(os.Stderr, "")

	// ── Step 2: Fund wallet ───────────────────────────────────────────────
	if !skipFundCheck {
		fmt.Fprintln(os.Stderr, "Step 2/5  Fund your preprod wallet")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "  Open the Cardano testnet faucet and request tADA for your address:")
		fmt.Fprintln(os.Stderr, "  https://docs.cardano.org/cardano-testnets/tools/faucet/")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintf(os.Stderr, "  Your address: %s\n", address)
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "  Press Enter once the faucet request is submitted (funds arrive in ~30s)...")

		reader := bufio.NewReader(os.Stdin)
		reader.ReadString('\n')
	} else {
		fmt.Fprintln(os.Stderr, "Step 2/5  Fund wallet (skipped — --skip-fund-check)")
	}
	fmt.Fprintln(os.Stderr, "")

	// ── Step 3: Blockfrost project ID ────────────────────────────────────
	fmt.Fprintln(os.Stderr, "Step 3/5  Configure Blockfrost submit endpoint")

	blockfrostID := blockfrostFlag
	if blockfrostID == "" {
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "  Create a free account at https://blockfrost.io, create a preprod project,")
		fmt.Fprintln(os.Stderr, "  and paste your project ID below.")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprint(os.Stderr, "  Blockfrost preprod project ID: ")

		reader := bufio.NewReader(os.Stdin)
		line, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read Blockfrost project ID: %w", err)
		}
		blockfrostID = strings.TrimSpace(line)
		if blockfrostID == "" {
			return fmt.Errorf("Blockfrost project ID is required\n\nCreate a free account at https://blockfrost.io")
		}
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	cfg.SubmitURL = "https://cardano-preprod.blockfrost.io/api/v0/tx/submit"
	if cfg.SubmitHeaders == nil {
		cfg.SubmitHeaders = make(map[string]string)
	}
	cfg.SubmitHeaders["project_id"] = blockfrostID

	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Fprintln(os.Stderr, "  ✓ Submit URL configured (Blockfrost preprod)")
	fmt.Fprintln(os.Stderr, "")

	// ── Step 4: Authenticate headlessly ──────────────────────────────────
	fmt.Fprintln(os.Stderr, "Step 4/5  Authenticate with your keypair")

	alias := aliasFlag
	if alias == "" {
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "  Choose an Andamio alias. This is your on-chain identity.")
		fmt.Fprintln(os.Stderr, "  Lowercase letters, numbers, and hyphens only (e.g. alice-dev-01).")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprint(os.Stderr, "  Alias: ")

		reader := bufio.NewReader(os.Stdin)
		line, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read alias: %w", err)
		}
		alias = strings.TrimSpace(line)
		if alias == "" {
			return fmt.Errorf("alias is required")
		}
	}

	// Re-load config after save above
	cfg, err = config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if err := runHeadlessLogin(cmd.Context(), cfg, skeyPath, alias, address); err != nil {
		return fmt.Errorf("authentication failed: %w\n\nMake sure your wallet is funded and try again.", err)
	}

	fmt.Fprintf(os.Stderr, "  ✓ Authenticated as: %s\n", alias)
	fmt.Fprintln(os.Stderr, "")

	// ── Step 5: Create course + project on-chain ──────────────────────────
	fmt.Fprintln(os.Stderr, "Step 5/5  Creating course and project on-chain")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "  This submits two Cardano transactions. Each takes ~30-60s to confirm.")
	fmt.Fprintln(os.Stderr, "")

	// Re-load config to get fresh JWT
	cfg, err = config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	c := client.New(cfg)

	// Create course on-chain
	fmt.Fprintln(os.Stderr, "  Creating course...")
	courseID, err := createOnChainInstance(cmd.Context(), c, cfg, skeyPath, alias, "course")
	if err != nil {
		return fmt.Errorf("failed to create course on-chain: %w", err)
	}
	fmt.Fprintf(os.Stderr, "  ✓ Course created: %s\n", courseID)

	// Register course metadata
	if err := registerCourseMetadata(cmd.Context(), c, courseID, courseTitle); err != nil {
		fmt.Fprintf(os.Stderr, "  ⚠ Course on-chain but metadata registration failed: %v\n", err)
		fmt.Fprintf(os.Stderr, "    Run manually: andamio course owner register --course-id %s --title %q\n", courseID, courseTitle)
	} else {
		fmt.Fprintf(os.Stderr, "  ✓ Course metadata registered\n")
	}

	fmt.Fprintln(os.Stderr, "")

	// Create project on-chain
	fmt.Fprintln(os.Stderr, "  Creating project...")
	projectID, err := createOnChainInstance(cmd.Context(), c, cfg, skeyPath, alias, "project")
	if err != nil {
		return fmt.Errorf("failed to create project on-chain: %w", err)
	}
	fmt.Fprintf(os.Stderr, "  ✓ Project created: %s\n", projectID)

	// Register project metadata
	if err := registerProjectMetadata(cmd.Context(), c, projectID, projectTitle); err != nil {
		fmt.Fprintf(os.Stderr, "  ⚠ Project on-chain but metadata registration failed: %v\n", err)
		fmt.Fprintf(os.Stderr, "    Run manually: andamio project owner register --project-id %s --title %q\n", projectID, projectTitle)
	} else {
		fmt.Fprintf(os.Stderr, "  ✓ Project metadata registered\n")
	}

	// ── Done ──────────────────────────────────────────────────────────────
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "┌─────────────────────────────────────────┐")
	fmt.Fprintln(os.Stderr, "│              Setup Complete             │")
	fmt.Fprintln(os.Stderr, "└─────────────────────────────────────────┘")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintf(os.Stderr, "  Alias:      %s\n", alias)
	fmt.Fprintf(os.Stderr, "  Course ID:  %s\n", courseID)
	fmt.Fprintf(os.Stderr, "  Project ID: %s\n", projectID)
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "  Next steps:")
	fmt.Fprintln(os.Stderr, "    andamio course get "+courseID)
	fmt.Fprintln(os.Stderr, "    andamio project get "+projectID)
	fmt.Fprintln(os.Stderr, "    andamio user me")
	fmt.Fprintln(os.Stderr, "")

	// Emit IDs to stdout for scripting (npm run seed can capture these)
	result := map[string]string{
		"alias":      alias,
		"course_id":  courseID,
		"project_id": projectID,
	}
	b, _ := json.Marshal(result)
	fmt.Println(string(b))

	return nil
}

// generateAndSaveKeypair generates a fresh Cardano ed25519 keypair using bursa,
// writes payment.skey, payment.vkey, and payment.addr to keysDir, and returns
// the skey path and bech32 address.
func generateAndSaveKeypair(keysDir string) (skeyPath, address string, err error) {
	mnemonic, err := bursa.GenerateMnemonic()
	if err != nil {
		return "", "", fmt.Errorf("failed to generate mnemonic: %w", err)
	}

	wallet, err := bursa.NewWallet(mnemonic, bursa.WithNetwork("testnet"))
	if err != nil {
		return "", "", fmt.Errorf("failed to create wallet: %w", err)
	}

	if err := os.MkdirAll(keysDir, 0700); err != nil {
		return "", "", fmt.Errorf("failed to create keys directory: %w", err)
	}

	// Write payment.skey
	skeyPath = keysDir + "/payment.skey"
	skeyData, err := json.MarshalIndent(wallet.PaymentSKey, "", "  ")
	if err != nil {
		return "", "", fmt.Errorf("failed to marshal skey: %w", err)
	}
	if err := os.WriteFile(skeyPath, skeyData, 0600); err != nil {
		return "", "", fmt.Errorf("failed to write payment.skey: %w", err)
	}

	// Write payment.vkey
	vkeyPath := keysDir + "/payment.vkey"
	vkeyData, err := json.MarshalIndent(wallet.PaymentVKey, "", "  ")
	if err != nil {
		return "", "", fmt.Errorf("failed to marshal vkey: %w", err)
	}
	if err := os.WriteFile(vkeyPath, vkeyData, 0600); err != nil {
		return "", "", fmt.Errorf("failed to write payment.vkey: %w", err)
	}

	// Write payment.addr
	addrPath := keysDir + "/payment.addr"
	if err := os.WriteFile(addrPath, []byte(wallet.PaymentAddress), 0600); err != nil {
		return "", "", fmt.Errorf("failed to write payment.addr: %w", err)
	}

	return skeyPath, wallet.PaymentAddress, nil
}

// createOnChainInstance runs the full tx lifecycle to create a course or project on-chain.
// instanceType is "course" or "project". Returns the instance ID from the build response.
func createOnChainInstance(ctx context.Context, c *client.Client, cfg *config.Config, skeyPath, alias, instanceType string) (string, error) {
	var endpoint string
	var txType string
	var idField string

	switch instanceType {
	case "course":
		endpoint = "/v2/tx/instance/owner/course/create"
		txType = "course_create"
		idField = "course_id"
	case "project":
		endpoint = "/v2/tx/instance/owner/project/create"
		txType = "project_create"
		idField = "project_id"
	default:
		return "", fmt.Errorf("unknown instance type: %s", instanceType)
	}

	body := map[string]interface{}{
		"alias": alias,
	}

	privKey, pubKey, err := cardano.LoadSigningKey(skeyPath)
	if err != nil {
		return "", fmt.Errorf("failed to load signing key: %w", err)
	}

	result, err := executeTxLifecycle(ctx, c, cfg, TxLifecycleParams{
		Endpoint: endpoint,
		Body:     body,
		SkeyPath: skeyPath,
		TxType:   txType,
	})
	if err != nil {
		return "", err
	}

	// Extract the instance ID from the build response
	if result.BuildResponse != nil {
		if id, ok := result.BuildResponse[idField].(string); ok && id != "" {
			return id, nil
		}
	}

	// Fallback: derive from tx hash + public key hash (not ideal but recoverable)
	_ = privKey
	_ = pubKey
	if result.TxHash != "" {
		return "", fmt.Errorf("%s created (tx: %s) but could not extract %s from response — check 'andamio %s owner list'",
			instanceType, result.TxHash, idField, instanceType)
	}

	return "", fmt.Errorf("transaction completed but %s not found in response", idField)
}

// registerCourseMetadata registers off-chain metadata for a course.
func registerCourseMetadata(ctx context.Context, c *client.Client, courseID, title string) error {
	payload := map[string]interface{}{
		"course_id":   courseID,
		"title":       title,
		"description": "Created with andamio init",
		"is_public":   true,
	}
	var resp map[string]interface{}
	return c.Post(ctx, "/api/v2/course/owner/course/register", payload, &resp)
}

// registerProjectMetadata registers off-chain metadata for a project.
func registerProjectMetadata(ctx context.Context, c *client.Client, projectID, title string) error {
	payload := map[string]interface{}{
		"project_id":  projectID,
		"title":       title,
		"description": "Created with andamio init",
		"is_public":   true,
	}
	var resp map[string]interface{}
	return c.Post(ctx, "/api/v2/project/owner/project/register", payload, &resp)
}
