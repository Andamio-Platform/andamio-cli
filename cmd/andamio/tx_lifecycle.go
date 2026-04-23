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
func executeTxLifecycle(ctx context.Context, c *client.Client, cfg *config.Config, params TxLifecycleParams) (*RunResult, error) {
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

	// Set up a derived context for this lifecycle, rooted in the caller's ctx.
	// Cancelling the parent (root SIGINT) or the tx-specific signal handler
	// below both propagate to the HTTP calls.
	ctx, cancel := context.WithCancel(ctx)
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
	if err := c.Post(ctx, "/api"+params.Endpoint, params.Body, &buildResp); err != nil {
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

	// Auto-inject task_hash into metadata for project TX types that need it.
	// The gateway's confirm logic requires task_hash for project_join and
	// project_credential_claim but andamioscan doesn't include it in the
	// event response. Extract it from the build request body.
	metadata := params.Metadata
	if _, hasTaskHash := metadata["task_hash"]; !hasTaskHash {
		if th := extractTaskHash(ctx, params.Body, params.TxType, c); th != "" {
			if metadata == nil {
				metadata = make(map[string]string)
			}
			metadata["task_hash"] = th
		}
	}
	if len(metadata) > 0 {
		registerPayload["metadata"] = metadata
	}

	var registerResp map[string]interface{}
	if err := c.Post(ctx, "/api/v2/tx/register", registerPayload, &registerResp); err != nil {
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
			if finalState == "failed" {
				fmt.Fprintf(os.Stderr, "\nTransaction confirmed on-chain but DB update failed.\n")
				fmt.Fprintf(os.Stderr, "The on-chain state is authoritative — the gateway will retry automatically.\n")
				fmt.Fprintf(os.Stderr, "\nDiagnostics:\n")
				fmt.Fprintf(os.Stderr, "  andamio tx status %s\n", signResult.TxHash)
				fmt.Fprintf(os.Stderr, "\nIf the state remains 'failed', re-register to retry:\n")
				fmt.Fprintf(os.Stderr, "  andamio tx register --tx-hash %s --tx-type %s\n", signResult.TxHash, params.TxType)
			} else {
				fmt.Fprintf(os.Stderr, "\nCheck later: andamio tx status %s\n", signResult.TxHash)
			}
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

// extractTaskHash pulls task_hash from the request body for project TX types
// that need it in registration metadata. Returns empty string if not applicable.
//
// The gateway's confirm logic requires task_hash for project_join and
// project_credential_claim. For project_join, task_hash is in the request body.
// For project_credential_claim, it's not — we look it up from the contributor's
// active commitment via the API.
func extractTaskHash(ctx context.Context, body interface{}, txType string, c *client.Client) string {
	m, ok := body.(map[string]interface{})
	if !ok {
		return ""
	}

	switch txType {
	case "project_join", "task_submit", "task_assess":
		// task_hash is directly in the request body
		if th, ok := m["task_hash"].(string); ok && th != "" {
			return th
		}
		return ""

	case "project_credential_claim":
		// task_hash isn't in the credential claim body, but we can look it up
		// from the contributor's commitment for this project
		projectID, _ := m["project_id"].(string)
		if projectID == "" || c == nil {
			return ""
		}
		return lookupContributorTaskHash(ctx, c, projectID)

	default:
		return ""
	}
}

// lookupContributorTaskHash fetches the contributor's commitments and returns
// the task_hash of the first ACCEPTED commitment for the given project.
// Falls back to the contributor's on-chain commitments if DB records don't match.
func lookupContributorTaskHash(ctx context.Context, c *client.Client, projectID string) string {
	// Try DB commitments first (merged records with status)
	var resp []interface{}
	if err := c.Post(ctx, "/api/v2/project/contributor/commitments/list", nil, &resp); err == nil {
		for _, item := range resp {
			commitment, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			pid, _ := commitment["project_id"].(string)
			if pid != projectID {
				continue
			}
			content, _ := commitment["content"].(map[string]interface{})
			if content != nil {
				status, _ := content["commitment_status"].(string)
				if status == "ACCEPTED" {
					if th, ok := commitment["task_hash"].(string); ok && th != "" {
						return th
					}
				}
			}
			// Also check chain_only commitments (no content/status, just task_hash)
			if th, ok := commitment["task_hash"].(string); ok && th != "" {
				source, _ := commitment["source"].(string)
				if source == "chain_only" {
					return th
				}
			}
		}
	}

	// Fallback: get task_hash from the project's task list
	var taskResp map[string]interface{}
	body := map[string]string{"project_id": projectID}
	if err := c.Post(ctx, "/api/v2/project/user/tasks/list", body, &taskResp); err == nil {
		if data, ok := taskResp["data"].([]interface{}); ok {
			for _, item := range data {
				task, ok := item.(map[string]interface{})
				if !ok {
					continue
				}
				if th, ok := task["task_hash"].(string); ok && th != "" {
					return th
				}
			}
		}
	}

	return ""
}
