package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/Andamio-Platform/andamio-cli/internal/apierr"
	"github.com/Andamio-Platform/andamio-cli/internal/cardano"
	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/Andamio-Platform/andamio-cli/internal/config"
	"github.com/Andamio-Platform/andamio-cli/internal/output"
	"github.com/Andamio-Platform/andamio-cli/internal/submit"
)

// TxLifecycleParams contains the parameters for executeTxLifecycle.
type TxLifecycleParams struct {
	Endpoint   string
	Body       interface{}
	SkeyPath   string
	TxType     string
	InstanceID string
	Metadata   map[string]string
	NoWait     bool
	Timeout    time.Duration
	SubmitURL  string
	Headers    []string
}

// executeTxLifecycle runs the full Cardano transaction lifecycle:
// build -> sign -> submit -> register -> poll.
// It returns the RunResult with partial progress on any failure.
func executeTxLifecycle(c *client.Client, cfg *config.Config, params TxLifecycleParams) (*RunResult, error) {
	isJSON := output.GetFormat() == output.FormatJSON

	// Resolve submit URL: params > config > error
	submitURL := params.SubmitURL
	if submitURL == "" {
		submitURL = cfg.SubmitURL
	}
	if submitURL == "" {
		return nil, fmt.Errorf("no submit URL configured\n\nSet one with:\n  andamio config set-submit-url <url>\n\nOr pass --submit-url <url>")
	}
	if err := config.ValidateSubmitURL(submitURL); err != nil {
		return nil, err
	}

	// Merge config headers with flag headers (flag headers take precedence)
	mergedHeaders := mergeSubmitHeaders(cfg.SubmitHeaders, params.Headers)

	// Set up context with SIGINT handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var mu sync.Mutex
	result := &RunResult{TxType: params.TxType}

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
		if txHash != "" {
			fmt.Fprintf(os.Stderr, "\nInterrupted. Transaction may have been submitted. Check: andamio tx status %s\n", txHash)
		}
		cancel()
		os.Exit(1)
	}()

	// --- Step 1: Build ---
	mu.Lock()
	result.Step = "build"
	mu.Unlock()
	if !isJSON {
		fmt.Fprintf(os.Stderr, "  Building transaction: POST %s\n", params.Endpoint)
	}

	var buildResp map[string]interface{}
	if err := c.Post("/api"+params.Endpoint, params.Body, &buildResp); err != nil {
		return result, fail("build_failed", "build failed", err)
	}

	mu.Lock()
	result.BuildResponse = buildResp
	mu.Unlock()

	unsignedTx, ok := buildResp["unsigned_tx"].(string)
	if !ok || unsignedTx == "" {
		return result, fail("build_failed", "build failed", fmt.Errorf("response missing unsigned_tx field"))
	}

	if !isJSON {
		fmt.Fprintf(os.Stderr, "  \u2713 Built unsigned TX\n")
	}

	// --- Step 2: Sign ---
	mu.Lock()
	result.Step = "sign"
	mu.Unlock()

	privKey, pubKey, err := cardano.LoadSigningKey(params.SkeyPath)
	if err != nil {
		return result, fail("sign_failed", "sign failed", err)
	}

	signResult, err := cardano.SignTransaction(unsignedTx, privKey, pubKey)
	if err != nil {
		return result, fail("sign_failed", "sign failed", err)
	}

	mu.Lock()
	result.TxHash = signResult.TxHash
	mu.Unlock()
	if !isJSON {
		fmt.Fprintf(os.Stderr, "  \u2713 Signed with %s (tx: %s...)\n", params.SkeyPath, signResult.TxHash[:8])
	}

	// --- Step 3: Submit ---
	mu.Lock()
	result.Step = "submit"
	mu.Unlock()
	if !isJSON {
		fmt.Fprintf(os.Stderr, "  Submitting to %s\n", submitURL)
	}

	if _, err := submit.SubmitTransaction(submitURL, signResult.SignedTx, mergedHeaders); err != nil {
		if !isJSON {
			fmt.Fprintf(os.Stderr, "  TX hash: %s\n", signResult.TxHash)
		}
		return result, fail("submit_failed", "submit failed (tx may be in mempool)", err)
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
		"tx_type": params.TxType,
	}
	if params.InstanceID != "" {
		registerPayload["instance_id"] = params.InstanceID
	}
	if len(params.Metadata) > 0 {
		registerPayload["metadata"] = params.Metadata
	}

	var registerResp map[string]interface{}
	if err := c.Post("/api/v2/tx/register", registerPayload, &registerResp); err != nil {
		if !isJSON {
			fmt.Fprintf(os.Stderr, "  Warning: registration failed but TX is on-chain.\n")
			fmt.Fprintf(os.Stderr, "  TX hash: %s\n", signResult.TxHash)
			fmt.Fprintf(os.Stderr, "  Recovery: andamio tx register --tx-hash %s --tx-type %s\n", signResult.TxHash, params.TxType)
		}
		return result, fail("register_failed", "register failed", err)
	}

	if !isJSON {
		fmt.Fprintf(os.Stderr, "  \u2713 Registered as %s\n", params.TxType)
	}

	// --- Step 5: Wait for confirmation ---
	if params.NoWait {
		mu.Lock()
		result.Step = "registered"
		result.State = "registered"
		mu.Unlock()
		if isJSON {
			return result, output.PrintJSON(result)
		}
		fmt.Fprintf(os.Stderr, "  TX hash: %s\n", signResult.TxHash)
		fmt.Fprintf(os.Stderr, "\nNext: andamio tx status %s\n", signResult.TxHash)
		return result, nil
	}

	mu.Lock()
	result.Step = "polling"
	mu.Unlock()
	if !isJSON {
		fmt.Fprintf(os.Stderr, "  \u23f3 Waiting for confirmation...\n")
	}

	finalState, err := pollTxStatus(ctx, c, signResult.TxHash, params.Timeout, isJSON)
	if err != nil {
		if !isJSON {
			fmt.Fprintf(os.Stderr, "  TX hash: %s\n", signResult.TxHash)
			fmt.Fprintf(os.Stderr, "\nCheck later: andamio tx status %s\n", signResult.TxHash)
		}
		return result, fail(finalState, "poll failed", err)
	}

	mu.Lock()
	result.State = finalState
	result.Step = "complete"
	mu.Unlock()

	if !isJSON {
		fmt.Fprintf(os.Stderr, "  \u2713 Confirmed on-chain\n")
		fmt.Fprintf(os.Stderr, "  \u2713 DB updated \u2014 complete!\n")
	}

	return result, nil
}
