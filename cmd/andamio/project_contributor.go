package main

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/Andamio-Platform/andamio-cli/internal/config"
	"github.com/Andamio-Platform/andamio-cli/internal/output"
	"github.com/spf13/cobra"
)

var projectContributorCmd = &cobra.Command{
	Use:               "contributor",
	Short:             "Project contributor operations (requires user login)",
	PersistentPreRunE: jwtAuthPreRunE,
}

var projectContributorListCmd = &cobra.Command{
	Use:   "list",
	Short: "List projects you contribute to",
	RunE: func(cmd *cobra.Command, args []string) error {
		return printList(
			"/api/v2/project/contributor/projects/list",
			"No contributor projects found.",
			"content.title", "project_id", true,
		)
	},
}

var projectContributorCommitmentsCmd = &cobra.Command{
	Use:   "commitments",
	Short: "List your task commitments",
	RunE: func(cmd *cobra.Command, args []string) error {
		return printList(
			"/api/v2/project/contributor/commitments/list",
			"No commitments found.",
			"content.title", "commitment_id", true,
		)
	},
}

var projectContributorCommitmentCmd = &cobra.Command{
	Use:   "commitment",
	Short: "Get a specific task commitment",
	Long: `Get details for a specific task commitment.

Examples:
  andamio project contributor commitment --project-id <id> --task-index 3`,
	RunE: runProjectContributorCommitment,
}

var projectContributorCommitCmd = &cobra.Command{
	Use:   "commit",
	Short: "Commit to a task",
	Long: `Create a new commitment to a project task.

Examples:
  andamio project contributor commit --project-id <id> --task-index 3`,
	RunE: runTaskHashAction("/api/v2/project/contributor/commitment/create", "Committing to task"),
}

var projectContributorUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update task commitment evidence",
	Long: `Update the evidence for your task commitment.

Examples:
  andamio project contributor update --project-id <id> --task-index 3 --evidence "https://github.com/..."`,
	RunE: runProjectContributorUpdate,
}

var projectContributorDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete a task commitment",
	Long: `Withdraw your commitment to a project task.

Examples:
  andamio project contributor delete --project-id <id> --task-index 3`,
	RunE: runTaskHashAction("/api/v2/project/contributor/commitment/delete", "Deleting commitment"),
}

var projectContributorCommitTxCmd = &cobra.Command{
	Use:   "commit-tx",
	Short: "On-chain task commitment with evidence",
	Long: `Build, sign, submit, and register an on-chain task commitment transaction.

Combines evidence preparation with the full Cardano transaction lifecycle.
Accepts markdown evidence, converts to Tiptap JSON, computes Blake2b-256 hash,
and uses the hash as task_info in the on-chain transaction.

After the on-chain tx, submits evidence to the off-chain API with the pending tx hash.
Evidence is optional — omit --evidence/--evidence-file for a commit-only transaction.

Examples:
  andamio project contributor commit-tx \
    --project-id <id> --task-index 0 \
    --evidence-file ./evidence.md --skey ./payment.skey

  andamio project contributor commit-tx \
    --project-id <id> --task-index 0 \
    --evidence "See https://github.com/my/pr" --skey ./payment.skey

  # Commit without evidence (on-chain only)
  andamio project contributor commit-tx \
    --project-id <id> --task-index 0 --skey ./payment.skey`,
	RunE: runProjectContributorCommitTx,
}

func init() {
	projectCmd.AddCommand(projectContributorCmd)
	projectContributorCmd.AddCommand(projectContributorListCmd)
	projectContributorCmd.AddCommand(projectContributorCommitmentsCmd)
	projectContributorCmd.AddCommand(projectContributorCommitmentCmd)
	projectContributorCmd.AddCommand(projectContributorCommitCmd)
	projectContributorCmd.AddCommand(projectContributorUpdateCmd)
	projectContributorCmd.AddCommand(projectContributorDeleteCmd)
	projectContributorCmd.AddCommand(projectContributorCommitTxCmd)

	// commit-tx flags
	projectContributorCommitTxCmd.Flags().String("project-id", "", "Project ID (required)")
	projectContributorCommitTxCmd.MarkFlagRequired("project-id")
	projectContributorCommitTxCmd.Flags().Int("task-index", -1, "Task index (required)")
	projectContributorCommitTxCmd.MarkFlagRequired("task-index")
	projectContributorCommitTxCmd.Flags().String("skey", "", "Path to Cardano .skey file for signing (required)")
	projectContributorCommitTxCmd.MarkFlagRequired("skey")
	projectContributorCommitTxCmd.Flags().String("evidence", "", "Evidence text or URL (Markdown supported)")
	projectContributorCommitTxCmd.Flags().String("evidence-file", "", "Path to evidence file (Markdown)")
	projectContributorCommitTxCmd.Flags().String("submit-url", "", "Override submit API URL")
	projectContributorCommitTxCmd.Flags().StringArray("submit-header", nil, "Additional submit headers (repeatable)")
	projectContributorCommitTxCmd.Flags().Bool("no-wait", false, "Exit after registration without polling")
	projectContributorCommitTxCmd.Flags().Duration("timeout", 10*time.Minute, "Max time to wait for confirmation")

	// Shared flags for task-specific commands
	for _, cmd := range []*cobra.Command{
		projectContributorCommitmentCmd,
		projectContributorCommitCmd,
		projectContributorDeleteCmd,
	} {
		cmd.Flags().String("project-id", "", "Project ID (required)")
		cmd.MarkFlagRequired("project-id")
		cmd.Flags().String("task-index", "", "Task index (required)")
		cmd.MarkFlagRequired("task-index")
	}

	// Update flags (add --evidence / --evidence-file)
	projectContributorUpdateCmd.Flags().String("project-id", "", "Project ID (required)")
	projectContributorUpdateCmd.MarkFlagRequired("project-id")
	projectContributorUpdateCmd.Flags().String("task-index", "", "Task index (required)")
	projectContributorUpdateCmd.MarkFlagRequired("task-index")
	projectContributorUpdateCmd.Flags().String("evidence", "", "Evidence text or URL (Markdown supported)")
	projectContributorUpdateCmd.Flags().String("evidence-file", "", "Path to evidence file (Markdown)")
}

// loadClientAndResolveTask loads config, creates a client, and resolves task_hash from project_id + task_index.
func loadClientAndResolveTask(cmd *cobra.Command) (*client.Client, string, int, error) {
	projectID, _ := cmd.Flags().GetString("project-id")
	taskIndexStr, _ := cmd.Flags().GetString("task-index")

	taskIndex, err := strconv.Atoi(taskIndexStr)
	if err != nil {
		return nil, "", 0, fmt.Errorf("invalid task-index %q: must be a number", taskIndexStr)
	}

	cfg, err := config.Load()
	if err != nil {
		return nil, "", 0, err
	}

	c := client.New(cfg)
	taskHash, err := resolveTaskHash(c, projectID, taskIndex)
	if err != nil {
		return nil, "", 0, err
	}
	return c, taskHash, taskIndex, nil
}

func runProjectContributorCommitment(cmd *cobra.Command, args []string) error {
	projectID, _ := cmd.Flags().GetString("project-id")

	c, taskHash, _, err := loadClientAndResolveTask(cmd)
	if err != nil {
		return err
	}

	payload := map[string]string{
		"project_id": projectID,
		"task_hash":  taskHash,
	}
	var resp map[string]interface{}
	if err := c.Post("/api/v2/project/contributor/commitment/get", payload, &resp); err != nil {
		return fmt.Errorf("failed to get commitment: %w", err)
	}

	return output.PrintJSON(resp)
}

// runTaskHashAction returns a RunE for commands that resolve task_hash and POST with it.
func runTaskHashAction(endpoint, verb string) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		isJSON := output.GetFormat() == output.FormatJSON

		c, taskHash, taskIndex, err := loadClientAndResolveTask(cmd)
		if err != nil {
			return err
		}

		if !isJSON {
			fmt.Fprintf(os.Stderr, "%s %d...\n", verb, taskIndex)
		}

		payload := map[string]interface{}{
			"task_hash": taskHash,
		}

		var resp map[string]interface{}
		if err := c.Post(endpoint, payload, &resp); err != nil {
			return fmt.Errorf("failed: %w", err)
		}

		if isJSON {
			return output.PrintJSON(resp)
		}

		fmt.Fprintf(os.Stderr, "Done.\n")
		return nil
	}
}

// runProjectContributorUpdate sends evidence as Tiptap JSON with a Blake2b-256 content hash.
func runProjectContributorUpdate(cmd *cobra.Command, args []string) error {
	isJSON := output.GetFormat() == output.FormatJSON

	evidence, err := readEvidenceFlag(cmd)
	if err != nil {
		return err
	}

	tiptapDoc, evidenceHash, err := wrapEvidence(evidence)
	if err != nil {
		return fmt.Errorf("failed to format evidence: %w", err)
	}

	c, taskHash, taskIndex, err := loadClientAndResolveTask(cmd)
	if err != nil {
		return err
	}

	if !isJSON {
		fmt.Fprintf(os.Stderr, "Updating commitment evidence for task %d...\n", taskIndex)
	}

	payload := map[string]interface{}{
		"task_hash":     taskHash,
		"evidence":      tiptapDoc,
		"evidence_hash": evidenceHash,
	}

	var resp map[string]interface{}
	if err := c.Post("/api/v2/project/contributor/commitment/update", payload, &resp); err != nil {
		return fmt.Errorf("failed to update commitment: %w", err)
	}

	if isJSON {
		return output.PrintJSON(resp)
	}

	fmt.Fprintf(os.Stderr, "Commitment updated.\n")
	return nil
}

// runProjectContributorCommitTx handles the full on-chain task commitment with evidence.
func runProjectContributorCommitTx(cmd *cobra.Command, args []string) error {
	projectID, _ := cmd.Flags().GetString("project-id")
	taskIndex, _ := cmd.Flags().GetInt("task-index")
	skeyPath, _ := cmd.Flags().GetString("skey")
	submitURL, _ := cmd.Flags().GetString("submit-url")
	headers, _ := cmd.Flags().GetStringArray("submit-header")
	noWait, _ := cmd.Flags().GetBool("no-wait")
	timeout, _ := cmd.Flags().GetDuration("timeout")
	isJSON := output.GetFormat() == output.FormatJSON

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	if err := checkJWTExpiry(cfg, isJSON); err != nil {
		return err
	}

	// Require wallet address from login
	if cfg.UserAddress == "" {
		return fmt.Errorf("no wallet address in config\n\nRe-login with your signing key to store your address:\n  andamio user login --skey <path> --alias <name>")
	}

	// Validate skey matches stored identity
	warnSkeyMismatch(skeyPath, cfg, isJSON)

	c := client.New(cfg)

	// Prepare evidence (optional)
	var tiptapDoc map[string]interface{}
	var evidenceHash string
	var offchainError string
	hasEvidence := cmd.Flags().Changed("evidence") || cmd.Flags().Changed("evidence-file")
	if hasEvidence {
		evidence, err := readEvidenceFlag(cmd)
		if err != nil {
			return err
		}
		if !isJSON {
			fmt.Fprintf(os.Stderr, "  Preparing evidence...\n")
		}
		tiptapDoc, evidenceHash, err = wrapEvidence(evidence)
		if err != nil {
			return fmt.Errorf("failed to format evidence: %w", err)
		}
		if !isJSON {
			fmt.Fprintf(os.Stderr, "  \u2713 Evidence hashed (blake2b-256: %s...)\n", evidenceHash[:16])
		}
	}

	// Resolve task_hash
	taskHash, err := resolveTaskHash(c, projectID, taskIndex)
	if err != nil {
		return err
	}
	if !isJSON {
		fmt.Fprintf(os.Stderr, "  \u2713 Resolved task_hash for task %d\n", taskIndex)
	}

	// Resolve contributor_state_id
	contributorStateID, err := resolveContributorStateID(c, projectID)
	if err != nil {
		return err
	}
	if !isJSON {
		fmt.Fprintf(os.Stderr, "  \u2713 Resolved contributor_state_id\n")
	}

	// Build the on-chain tx body
	buildBody := map[string]interface{}{
		"alias":                cfg.UserAlias,
		"project_id":           projectID,
		"task_hash":            taskHash,
		"contributor_state_id": contributorStateID,
		"initiator_data": map[string]interface{}{
			"change_address": cfg.UserAddress,
			"used_addresses": []string{cfg.UserAddress},
		},
	}
	if evidenceHash != "" {
		buildBody["task_info"] = evidenceHash
	}

	// Registration metadata
	metadata := map[string]string{
		"task_hash": taskHash,
	}
	if evidenceHash != "" {
		metadata["evidence_hash"] = evidenceHash
	}

	// Execute the tx lifecycle
	result, err := executeTxLifecycle(c, cfg, TxLifecycleParams{
		Endpoint:   "/v2/tx/project/contributor/task/commit",
		Body:       buildBody,
		SkeyPath:   skeyPath,
		TxType:     "task_submit",
		InstanceID: projectID,
		Metadata:   metadata,
		NoWait:     noWait,
		Timeout:    timeout,
		SubmitURL:  submitURL,
		Headers:    headers,
	})
	if err != nil {
		return err
	}

	// Off-chain evidence submission (only if evidence was provided and we have a tx hash)
	if hasEvidence && result.TxHash != "" {
		if !isJSON {
			fmt.Fprintf(os.Stderr, "  Submitting evidence to API...\n")
		}

		offchainPayload := map[string]interface{}{
			"task_hash":       taskHash,
			"evidence":        tiptapDoc,
			"evidence_hash":   evidenceHash,
			"pending_tx_hash": result.TxHash,
		}

		var offchainResp map[string]interface{}
		if offchainErr := c.Post("/api/v2/project/contributor/commitment/update", offchainPayload, &offchainResp); offchainErr != nil {
			// Warning only — the on-chain tx is already submitted
			offchainError = offchainErr.Error()
			if !isJSON {
				fmt.Fprintf(os.Stderr, "  Warning: off-chain evidence submission failed: %v\n", offchainErr)
				fmt.Fprintf(os.Stderr, "  Recovery: andamio project contributor update --project-id %s --task-index %d --evidence-file <path>\n", projectID, taskIndex)
			}
		} else if !isJSON {
			fmt.Fprintf(os.Stderr, "  \u2713 Evidence submitted to API\n")
		}
	}

	// Print final result in JSON mode (skip if noWait already printed via executeTxLifecycle)
	if isJSON && result.State != "registered" {
		commitResult := CommitTxResult{
			RunResult:     *result,
			EvidenceHash:  evidenceHash,
			TaskHash:      taskHash,
			OffchainError: offchainError,
		}
		return output.PrintJSON(commitResult)
	}

	return nil
}

