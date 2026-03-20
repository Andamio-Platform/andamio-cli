package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
	"time"

	"github.com/Andamio-Platform/andamio-cli/internal/apierr"
	"github.com/Andamio-Platform/andamio-cli/internal/cardano"
	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/Andamio-Platform/andamio-cli/internal/config"
	"github.com/Andamio-Platform/andamio-cli/internal/output"
	"github.com/Andamio-Platform/andamio-cli/internal/submit"
	"github.com/spf13/cobra"
)

// RunResult is the structured output for tx run, always populated to the extent known.
// On partial failure, Step indicates where it stopped and TxHash is included if signing completed.
type RunResult struct {
	TxHash        string                 `json:"tx_hash,omitempty"`
	TxType        string                 `json:"tx_type"`
	State         string                 `json:"state"`
	Step          string                 `json:"step"`
	BuildResponse map[string]interface{} `json:"build_response,omitempty"`
	Error         string                 `json:"error,omitempty"`
}

var txRunCmd = &cobra.Command{
	Use:   "run <endpoint>",
	Short: "Build, sign, submit, register, and confirm a transaction in one command",
	Long: `Execute the full Cardano transaction lifecycle in a single command.

Steps: build unsigned TX via API, sign with local .skey, submit to network,
register with Andamio state machine, and poll for DB confirmation.

All 5 existing tx commands (build, sign, submit, register, status) remain available
for advanced use and scripting. This command is a convenience layer on top.

Progress lines are printed to stderr. Use --output json for scripted consumption.
Use --no-wait to exit after registration without polling for confirmation.

Examples:
  andamio tx run /v2/tx/course/teacher/assignments/assess \
    --body '{"alias":"teacher-01","course_id":"abc123","assignment_decisions":[...]}' \
    --skey ./payment.skey \
    --tx-type assessment_assess

  andamio tx run /v2/tx/instance/owner/course/create \
    --body-file create-course.json \
    --skey ./payment.skey \
    --tx-type course_create \
    --instance-id abc123 \
    --no-wait`,
	Args: cobra.ExactArgs(1),
	PreRunE: func(cmd *cobra.Command, args []string) error {
		if err := rootCmd.PersistentPreRunE(cmd, args); err != nil {
			return err
		}
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		if !cfg.HasUserAuth() {
			return &apierr.AuthError{Message: "not authenticated. Run 'andamio user login' first"}
		}
		return nil
	},
	RunE: runTxRun,
}

func init() {
	txCmd.AddCommand(txRunCmd)
	txRunCmd.Flags().String("body", "", "Inline JSON request body")
	txRunCmd.Flags().String("body-file", "", "Path to JSON file (mutually exclusive with --body)")
	txRunCmd.Flags().String("skey", "", "Path to Cardano .skey file for signing")
	txRunCmd.Flags().String("tx-type", "", "Transaction type for registration (see 'andamio tx types')")
	txRunCmd.Flags().String("submit-url", "", "Override submit API URL (falls back to config)")
	txRunCmd.Flags().StringArray("submit-header", nil, "Additional submit headers (repeatable, format: \"Key: Value\")")
	txRunCmd.Flags().String("instance-id", "", "Course or project ID for registration")
	txRunCmd.Flags().StringArray("metadata", nil, "Metadata for registration (repeatable, format: key=value)")
	txRunCmd.Flags().Bool("no-wait", false, "Exit after registration without polling for confirmation")
	txRunCmd.Flags().Duration("timeout", 10*time.Minute, "Max time to wait for confirmation")
	txRunCmd.MarkFlagRequired("skey")
	txRunCmd.MarkFlagRequired("tx-type")
}

func runTxRun(cmd *cobra.Command, args []string) error {
	endpoint := args[0]
	bodyStr, _ := cmd.Flags().GetString("body")
	bodyFile, _ := cmd.Flags().GetString("body-file")
	skeyPath, _ := cmd.Flags().GetString("skey")
	txType, _ := cmd.Flags().GetString("tx-type")
	submitURL, _ := cmd.Flags().GetString("submit-url")
	headers, _ := cmd.Flags().GetStringArray("submit-header")
	instanceID, _ := cmd.Flags().GetString("instance-id")
	metadataFlags, _ := cmd.Flags().GetStringArray("metadata")
	noWait, _ := cmd.Flags().GetBool("no-wait")
	timeout, _ := cmd.Flags().GetDuration("timeout")
	isJSON := output.GetFormat() == output.FormatJSON

	// Validate body flags
	if bodyStr == "" && bodyFile == "" {
		return fmt.Errorf("either --body or --body-file is required")
	}
	if bodyStr != "" && bodyFile != "" {
		return fmt.Errorf("--body and --body-file are mutually exclusive")
	}

	// Parse metadata flags
	metadata, err := parseMetadataFlags(metadataFlags)
	if err != nil {
		return err
	}

	// Load body
	var bodyData interface{}
	if bodyFile != "" {
		data, err := os.ReadFile(bodyFile)
		if err != nil {
			return fmt.Errorf("failed to read body file: %w", err)
		}
		if err := json.Unmarshal(data, &bodyData); err != nil {
			return fmt.Errorf("invalid JSON in body file: %w", err)
		}
	} else {
		if err := json.Unmarshal([]byte(bodyStr), &bodyData); err != nil {
			return fmt.Errorf("invalid JSON in --body: %w", err)
		}
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	// JWT expiry pre-check
	if cfg.JWTExpiresAt != "" {
		expiresAt, err := time.Parse(time.RFC3339, cfg.JWTExpiresAt)
		if err == nil {
			remaining := time.Until(expiresAt)
			if remaining <= 0 {
				return &apierr.AuthError{Message: "JWT has expired. Run 'andamio user login' to refresh"}
			}
			if remaining < 5*time.Minute && !isJSON {
				fmt.Fprintf(os.Stderr, "Warning: JWT expires in %s — pipeline may fail at register step. Run 'andamio user login' to refresh.\n", remaining.Truncate(time.Second))
			}
		}
	}

	// Resolve submit URL: flag > config > error
	if submitURL == "" {
		submitURL = cfg.SubmitURL
	}
	if submitURL == "" {
		return fmt.Errorf("no submit URL configured\n\nSet one with:\n  andamio config set-submit-url <url>\n\nOr pass --submit-url <url>")
	}
	if err := config.ValidateSubmitURL(submitURL); err != nil {
		return err
	}

	// Set up context with SIGINT handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var mu sync.Mutex
	result := &RunResult{TxType: txType}

	// fail prints the RunResult as JSON (if in JSON mode) and returns the appropriate error.
	// In JSON mode it wraps the error as ReportedError so main.go sets the exit code
	// without printing a second error message.
	fail := func(state, msg string, origErr error) error {
		mu.Lock()
		result.State = state
		result.Error = origErr.Error()
		mu.Unlock()
		if isJSON {
			mu.Lock()
			_ = output.PrintJSON(result)
			mu.Unlock()
			return &apierr.ReportedError{Err: fmt.Errorf("%s: %w", msg, origErr)}
		}
		return fmt.Errorf("%s: %w", msg, origErr)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	defer signal.Stop(sigCh)
	go func() {
		<-sigCh
		mu.Lock()
		txHash := result.TxHash
		mu.Unlock()
		// Print recovery info. In a CLI, os.Exit is necessary here because
		// context cancellation alone cannot unblock synchronous HTTP calls
		// in the build/sign/submit steps (the client package does not accept
		// a context). The OS reclaims all resources on exit.
		if txHash != "" {
			fmt.Fprintf(os.Stderr, "\nInterrupted. Transaction may have been submitted. Check: andamio tx status %s\n", txHash)
		}
		cancel()
		os.Exit(1)
	}()

	c := client.New(cfg)

	// --- Step 1: Build ---
	mu.Lock()
	result.Step = "build"
	mu.Unlock()
	if !isJSON {
		fmt.Fprintf(os.Stderr, "  Building transaction: POST %s\n", endpoint)
	}

	var buildResp map[string]interface{}
	if err := c.Post("/api"+endpoint, bodyData, &buildResp); err != nil {
		return fail("build_failed", "build failed", err)
	}

	mu.Lock()
	result.BuildResponse = buildResp
	mu.Unlock()

	unsignedTx, ok := buildResp["unsigned_tx"].(string)
	if !ok || unsignedTx == "" {
		return fail("build_failed", "build failed", fmt.Errorf("response missing unsigned_tx field"))
	}

	if !isJSON {
		fmt.Fprintf(os.Stderr, "  \u2713 Built unsigned TX\n")
	}

	// --- Step 2: Sign ---
	mu.Lock()
	result.Step = "sign"
	mu.Unlock()

	privKey, pubKey, err := cardano.LoadSigningKey(skeyPath)
	if err != nil {
		return fail("sign_failed", "sign failed", err)
	}

	signResult, err := cardano.SignTransaction(unsignedTx, privKey, pubKey)
	if err != nil {
		return fail("sign_failed", "sign failed", err)
	}

	mu.Lock()
	result.TxHash = signResult.TxHash
	mu.Unlock()
	if !isJSON {
		fmt.Fprintf(os.Stderr, "  \u2713 Signed with %s (tx: %s...)\n", skeyPath, signResult.TxHash[:8])
	}

	// --- Step 3: Submit ---
	mu.Lock()
	result.Step = "submit"
	mu.Unlock()
	if !isJSON {
		fmt.Fprintf(os.Stderr, "  Submitting to %s\n", submitURL)
	}

	if _, err := submit.SubmitTransaction(submitURL, signResult.SignedTx, headers); err != nil {
		if !isJSON {
			fmt.Fprintf(os.Stderr, "  TX hash: %s\n", signResult.TxHash)
		}
		return fail("submit_failed", "submit failed (tx may be in mempool)", err)
	}

	if !isJSON {
		fmt.Fprintf(os.Stderr, "  \u2713 Submitted to network\n")
	}

	// --- Step 4: Register ---
	mu.Lock()
	result.Step = "register"
	mu.Unlock()

	registerPayload := map[string]interface{}{
		"tx_hash": signResult.TxHash,
		"tx_type": txType,
	}
	if instanceID != "" {
		registerPayload["instance_id"] = instanceID
	}
	if len(metadata) > 0 {
		registerPayload["metadata"] = metadata
	}

	var registerResp map[string]interface{}
	if err := c.Post("/api/v2/tx/register", registerPayload, &registerResp); err != nil {
		if !isJSON {
			fmt.Fprintf(os.Stderr, "  Warning: registration failed but TX is on-chain.\n")
			fmt.Fprintf(os.Stderr, "  TX hash: %s\n", signResult.TxHash)
			fmt.Fprintf(os.Stderr, "  Recovery: andamio tx register --tx-hash %s --tx-type %s\n", signResult.TxHash, txType)
		}
		return fail("register_failed", "register failed", err)
	}

	if !isJSON {
		fmt.Fprintf(os.Stderr, "  \u2713 Registered as %s\n", txType)
	}

	// --- Step 5: Wait for confirmation ---
	if noWait {
		mu.Lock()
		result.Step = "registered"
		result.State = "registered"
		mu.Unlock()
		if isJSON {
			return output.PrintJSON(result)
		}
		fmt.Fprintf(os.Stderr, "  TX hash: %s\n", signResult.TxHash)
		fmt.Fprintf(os.Stderr, "\nNext: andamio tx status %s\n", signResult.TxHash)
		return nil
	}

	mu.Lock()
	result.Step = "polling"
	mu.Unlock()
	if !isJSON {
		fmt.Fprintf(os.Stderr, "  \u23f3 Waiting for confirmation...\n")
	}

	finalState, err := pollTxStatus(ctx, c, signResult.TxHash, timeout, isJSON)
	if err != nil {
		if !isJSON {
			fmt.Fprintf(os.Stderr, "  TX hash: %s\n", signResult.TxHash)
			fmt.Fprintf(os.Stderr, "\nCheck later: andamio tx status %s\n", signResult.TxHash)
		}
		return fail(finalState, "poll failed", err)
	}

	mu.Lock()
	result.State = finalState
	result.Step = "complete"
	mu.Unlock()

	if isJSON {
		return output.PrintJSON(result)
	}

	fmt.Fprintf(os.Stderr, "  \u2713 Confirmed on-chain\n")
	fmt.Fprintf(os.Stderr, "  \u2713 DB updated \u2014 complete!\n")
	return nil
}

// pollTxStatus polls GET /api/v2/tx/status/{tx_hash} every 5s until a terminal state
// or timeout. Returns the final state and an error for non-success terminal states.
func pollTxStatus(ctx context.Context, c *client.Client, txHash string, timeout time.Duration, isJSON bool) (string, error) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	var deadline <-chan time.Time
	if timeout > 0 {
		timer := time.NewTimer(timeout)
		defer timer.Stop()
		deadline = timer.C
	}

	consecutiveErrors := 0
	lastState := ""
	statusPath := "/api/v2/tx/status/" + url.PathEscape(txHash)

	for {
		select {
		case <-ctx.Done():
			return lastState, fmt.Errorf("interrupted")
		case <-deadline:
			return lastState, fmt.Errorf("timed out waiting for confirmation (last state: %s)", lastState)
		case <-ticker.C:
			var resp map[string]interface{}
			if err := c.Get(statusPath, &resp); err != nil {
				consecutiveErrors++
				if !isJSON {
					fmt.Fprintf(os.Stderr, "  Warning: poll failed (%d/3): %v\n", consecutiveErrors, err)
				}
				if consecutiveErrors >= 3 {
					return lastState, fmt.Errorf("polling failed after 3 consecutive errors: %w", err)
				}
				continue
			}
			consecutiveErrors = 0

			state, _ := resp["state"].(string)
			if state != lastState && state != "" {
				lastState = state
				if !isJSON {
					fmt.Fprintf(os.Stderr, "  State: %s\n", state)
				}
			}

			switch state {
			case "updated":
				return state, nil
			case "failed":
				return state, fmt.Errorf("transaction confirmed but DB update failed")
			case "expired":
				return state, fmt.Errorf("transaction expired without confirmation")
			}
		}
	}
}

// parseMetadataFlags parses --metadata key=value flags into a map.
func parseMetadataFlags(flags []string) (map[string]string, error) {
	if len(flags) == 0 {
		return nil, nil
	}
	m := make(map[string]string, len(flags))
	for _, f := range flags {
		parts := strings.SplitN(f, "=", 2)
		if len(parts) != 2 || parts[0] == "" {
			return nil, fmt.Errorf("invalid --metadata format %q: expected key=value", f)
		}
		m[parts[0]] = parts[1]
	}
	return m, nil
}
