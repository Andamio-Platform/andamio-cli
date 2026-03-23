package main

import (
	"fmt"
	"os"
	"time"

	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/Andamio-Platform/andamio-cli/internal/config"
	"github.com/Andamio-Platform/andamio-cli/internal/output"
	"github.com/spf13/cobra"
)

var courseStudentCmd = &cobra.Command{
	Use:               "student",
	Short:             "Course student operations (requires user login)",
	PersistentPreRunE: jwtAuthPreRunE,
}

var courseStudentCoursesCmd = &cobra.Command{
	Use:   "courses",
	Short: "List courses you're enrolled in",
	RunE: func(cmd *cobra.Command, args []string) error {
		return printList(
			"/api/v2/course/student/courses/list",
			"No enrolled courses found.",
			"content.title", "course_id", true,
		)
	},
}

var courseStudentCredentialsCmd = &cobra.Command{
	Use:   "credentials",
	Short: "List your earned credentials",
	RunE: func(cmd *cobra.Command, args []string) error {
		return printList(
			"/api/v2/course/student/credentials/list",
			"No credentials found.",
			"content.title", "credential_id", true,
		)
	},
}

var courseStudentCommitmentsCmd = &cobra.Command{
	Use:   "commitments",
	Short: "List your assignment commitments",
	RunE: func(cmd *cobra.Command, args []string) error {
		return printList(
			"/api/v2/course/student/assignment-commitments/list",
			"No commitments found.",
			"content.title", "commitment_id", true,
		)
	},
}

var courseStudentCommitmentCmd = &cobra.Command{
	Use:   "commitment",
	Short: "Get a specific assignment commitment",
	Long: `Get details for a specific assignment commitment.

Examples:
  andamio course student commitment --course-id <id> --module-code 101`,
	RunE: runCourseStudentCommitment,
}

var courseStudentCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Enroll in a course module (create commitment)",
	Long: `Create a new assignment commitment — enrolls you in a course module.

Examples:
  andamio course student create --course-id <id> --module-code 101`,
	RunE: runCourseStudentAction("/api/v2/course/student/commitment/create", "Creating commitment"),
}

var courseStudentSubmitCmd = &cobra.Command{
	Use:   "submit",
	Short: "Submit assignment evidence",
	Long: `Submit evidence for your assignment commitment.

Examples:
  andamio course student submit --course-id <id> --module-code 101 --evidence "https://github.com/..."`,
	RunE: runCourseStudentSubmit,
}

var courseStudentUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update assignment evidence",
	Long: `Update the evidence for your assignment commitment.

Examples:
  andamio course student update --course-id <id> --module-code 101 --evidence "https://github.com/..."`,
	RunE: runCourseStudentUpdate,
}

var courseStudentLeaveCmd = &cobra.Command{
	Use:   "leave",
	Short: "Leave a course module commitment",
	Long: `Withdraw from a course module commitment.

Examples:
  andamio course student leave --course-id <id> --module-code 101`,
	RunE: runCourseStudentAction("/api/v2/course/student/commitment/leave", "Leaving commitment"),
}

var courseStudentClaimCmd = &cobra.Command{
	Use:   "claim",
	Short: "Claim your course credential",
	Long: `Claim the credential for a completed course module.

Examples:
  andamio course student claim --course-id <id> --module-code 101`,
	RunE: runCourseStudentAction("/api/v2/course/student/commitment/claim", "Claiming credential"),
}

var courseStudentCommitTxCmd = &cobra.Command{
	Use:   "commit-tx",
	Short: "On-chain assignment commitment with evidence",
	Long: `Build, sign, submit, and register an on-chain assignment commitment transaction.

Combines evidence preparation with the full Cardano transaction lifecycle.
Accepts markdown evidence, converts to Tiptap JSON, computes Blake2b-256 hash,
and uses the hash as assignment_info in the on-chain transaction.

After the on-chain tx, submits evidence to the off-chain API with the pending tx hash.
Evidence is optional — omit --evidence/--evidence-file for a commit-only transaction.

Examples:
  andamio course student commit-tx \
    --course-id <id> --module-code 101 \
    --evidence-file ./evidence.md --skey ./payment.skey

  andamio course student commit-tx \
    --course-id <id> --module-code 101 \
    --evidence "See https://github.com/my/pr" --skey ./payment.skey

  # Commit without evidence (enroll on-chain only)
  andamio course student commit-tx \
    --course-id <id> --module-code 101 --skey ./payment.skey`,
	RunE: runCourseStudentCommitTx,
}

func init() {
	courseCmd.AddCommand(courseStudentCmd)
	courseStudentCmd.AddCommand(courseStudentCoursesCmd)
	courseStudentCmd.AddCommand(courseStudentCredentialsCmd)
	courseStudentCmd.AddCommand(courseStudentCommitmentsCmd)
	courseStudentCmd.AddCommand(courseStudentCommitmentCmd)
	courseStudentCmd.AddCommand(courseStudentCreateCmd)
	courseStudentCmd.AddCommand(courseStudentSubmitCmd)
	courseStudentCmd.AddCommand(courseStudentUpdateCmd)
	courseStudentCmd.AddCommand(courseStudentLeaveCmd)
	courseStudentCmd.AddCommand(courseStudentClaimCmd)
	courseStudentCmd.AddCommand(courseStudentCommitTxCmd)

	// commit-tx flags
	courseStudentCommitTxCmd.Flags().String("course-id", "", "Course ID (required)")
	courseStudentCommitTxCmd.MarkFlagRequired("course-id")
	courseStudentCommitTxCmd.Flags().String("module-code", "", "Module code (required)")
	courseStudentCommitTxCmd.MarkFlagRequired("module-code")
	courseStudentCommitTxCmd.Flags().String("skey", "", "Path to Cardano .skey file for signing (required)")
	courseStudentCommitTxCmd.MarkFlagRequired("skey")
	courseStudentCommitTxCmd.Flags().String("evidence", "", "Evidence text or URL (Markdown supported)")
	courseStudentCommitTxCmd.Flags().String("evidence-file", "", "Path to evidence file (Markdown)")
	courseStudentCommitTxCmd.Flags().String("submit-url", "", "Override submit API URL")
	courseStudentCommitTxCmd.Flags().StringArray("submit-header", nil, "Additional submit headers (repeatable)")
	courseStudentCommitTxCmd.Flags().Bool("no-wait", false, "Exit after registration without polling")
	courseStudentCommitTxCmd.Flags().Duration("timeout", 10*time.Minute, "Max time to wait for confirmation")

	// Commitment get flags
	courseStudentCommitmentCmd.Flags().String("course-id", "", "Course ID (required)")
	courseStudentCommitmentCmd.MarkFlagRequired("course-id")
	courseStudentCommitmentCmd.Flags().String("module-code", "", "Module code (required)")
	courseStudentCommitmentCmd.MarkFlagRequired("module-code")

	// Shared flags for lifecycle commands
	for _, cmd := range []*cobra.Command{
		courseStudentCreateCmd,
		courseStudentLeaveCmd,
		courseStudentClaimCmd,
	} {
		cmd.Flags().String("course-id", "", "Course ID (required)")
		cmd.MarkFlagRequired("course-id")
		cmd.Flags().String("module-code", "", "Module code (required)")
		cmd.MarkFlagRequired("module-code")
	}

	// Submit/update flags (add --evidence / --evidence-file)
	for _, cmd := range []*cobra.Command{
		courseStudentSubmitCmd,
		courseStudentUpdateCmd,
	} {
		cmd.Flags().String("course-id", "", "Course ID (required)")
		cmd.MarkFlagRequired("course-id")
		cmd.Flags().String("module-code", "", "Module code (required)")
		cmd.MarkFlagRequired("module-code")
		cmd.Flags().String("evidence", "", "Evidence text or URL (Markdown supported)")
		cmd.Flags().String("evidence-file", "", "Path to evidence file (Markdown)")
	}
}

func runCourseStudentCommitment(cmd *cobra.Command, args []string) error {
	courseID, _ := cmd.Flags().GetString("course-id")
	moduleCode, _ := cmd.Flags().GetString("module-code")

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	c := client.New(cfg)
	payload := map[string]string{
		"course_id":          courseID,
		"course_module_code": moduleCode,
	}
	var resp map[string]interface{}
	if err := c.Post("/api/v2/course/student/assignment-commitment/get", payload, &resp); err != nil {
		return fmt.Errorf("failed to get commitment: %w", err)
	}

	return output.PrintJSON(resp)
}

// runCourseStudentAction returns a RunE for simple course-id + module-code POST commands.
func runCourseStudentAction(endpoint, verb string) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		courseID, _ := cmd.Flags().GetString("course-id")
		moduleCode, _ := cmd.Flags().GetString("module-code")
		isJSON := output.GetFormat() == output.FormatJSON

		payload := map[string]interface{}{
			"course_id":          courseID,
			"course_module_code": moduleCode,
		}

		cfg, err := config.Load()
		if err != nil {
			return err
		}

		if !isJSON {
			fmt.Fprintf(os.Stderr, "%s for module %s...\n", verb, moduleCode)
		}

		c := client.New(cfg)
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

// runCourseStudentSubmit handles evidence submission. Resolves slt_hash from module code
// per SubmitAssignmentCommitmentV2Request schema.
func runCourseStudentSubmit(cmd *cobra.Command, args []string) error {
	courseID, _ := cmd.Flags().GetString("course-id")
	moduleCode, _ := cmd.Flags().GetString("module-code")
	isJSON := output.GetFormat() == output.FormatJSON

	evidence, err := readEvidenceFlag(cmd)
	if err != nil {
		return err
	}

	tiptapDoc, evidenceHash, err := wrapEvidence(evidence)
	if err != nil {
		return fmt.Errorf("failed to format evidence: %w", err)
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	c := client.New(cfg)

	// Submit endpoint requires slt_hash (on-chain module identifier)
	sltHash, err := resolveSltHash(c, courseID, moduleCode)
	if err != nil {
		return err
	}

	if !isJSON {
		fmt.Fprintf(os.Stderr, "Submitting evidence for module %s...\n", moduleCode)
	}

	payload := map[string]interface{}{
		"course_id":     courseID,
		"slt_hash":      sltHash,
		"evidence":      tiptapDoc,
		"evidence_hash": evidenceHash,
	}

	var resp map[string]interface{}
	if err := c.Post("/api/v2/course/student/commitment/submit", payload, &resp); err != nil {
		return fmt.Errorf("failed: %w", err)
	}

	if isJSON {
		return output.PrintJSON(resp)
	}

	fmt.Fprintf(os.Stderr, "Done.\n")
	return nil
}

// runCourseStudentUpdate handles evidence updates per UpdateAssignmentCommitmentV2Request schema.
func runCourseStudentUpdate(cmd *cobra.Command, args []string) error {
	courseID, _ := cmd.Flags().GetString("course-id")
	moduleCode, _ := cmd.Flags().GetString("module-code")
	isJSON := output.GetFormat() == output.FormatJSON

	evidence, err := readEvidenceFlag(cmd)
	if err != nil {
		return err
	}

	tiptapDoc, evidenceHash, err := wrapEvidence(evidence)
	if err != nil {
		return fmt.Errorf("failed to format evidence: %w", err)
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	if !isJSON {
		fmt.Fprintf(os.Stderr, "Updating evidence for module %s...\n", moduleCode)
	}

	c := client.New(cfg)

	payload := map[string]interface{}{
		"course_id":          courseID,
		"course_module_code": moduleCode,
		"evidence":           tiptapDoc,
		"evidence_hash":      evidenceHash,
	}

	var resp map[string]interface{}
	if err := c.Post("/api/v2/course/student/commitment/update", payload, &resp); err != nil {
		return fmt.Errorf("failed: %w", err)
	}

	if isJSON {
		return output.PrintJSON(resp)
	}

	fmt.Fprintf(os.Stderr, "Done.\n")
	return nil
}

// CommitTxResult extends RunResult with domain-specific fields for commit-tx commands.
type CommitTxResult struct {
	RunResult
	EvidenceHash  string `json:"evidence_hash,omitempty"`
	SltHash       string `json:"slt_hash,omitempty"`
	TaskHash      string `json:"task_hash,omitempty"`
	OffchainError string `json:"offchain_error,omitempty"`
}

// runCourseStudentCommitTx handles the full on-chain assignment commitment with evidence.
func runCourseStudentCommitTx(cmd *cobra.Command, args []string) error {
	courseID, _ := cmd.Flags().GetString("course-id")
	moduleCode, _ := cmd.Flags().GetString("module-code")
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

	// Resolve slt_hash
	sltHash, err := resolveSltHash(c, courseID, moduleCode)
	if err != nil {
		return err
	}
	if !isJSON {
		fmt.Fprintf(os.Stderr, "  \u2713 Resolved slt_hash for module %s\n", moduleCode)
	}

	// Build the on-chain tx body
	buildBody := map[string]interface{}{
		"alias":     cfg.UserAlias,
		"course_id": courseID,
		"slt_hash":  sltHash,
		"initiator_data": map[string]interface{}{
			"change_address": cfg.UserAddress,
			"used_addresses": []string{cfg.UserAddress},
		},
	}
	if evidenceHash != "" {
		buildBody["assignment_info"] = evidenceHash
	}

	// Registration metadata
	metadata := map[string]string{
		"slt_hash":           sltHash,
		"course_module_code": moduleCode,
	}
	if evidenceHash != "" {
		metadata["evidence_hash"] = evidenceHash
	}

	// Execute the tx lifecycle
	result, err := executeTxLifecycle(c, cfg, TxLifecycleParams{
		Endpoint:   "/v2/tx/course/student/assignment/commit",
		Body:       buildBody,
		SkeyPath:   skeyPath,
		TxType:     "assignment_submit",
		InstanceID: courseID,
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
			"course_id":       courseID,
			"slt_hash":        sltHash,
			"evidence":        tiptapDoc,
			"evidence_hash":   evidenceHash,
			"pending_tx_hash": result.TxHash,
		}

		var offchainResp map[string]interface{}
		if offchainErr := c.Post("/api/v2/course/student/commitment/submit", offchainPayload, &offchainResp); offchainErr != nil {
			// Warning only — the on-chain tx is already submitted
			offchainError = offchainErr.Error()
			if !isJSON {
				fmt.Fprintf(os.Stderr, "  Warning: off-chain evidence submission failed: %v\n", offchainErr)
				fmt.Fprintf(os.Stderr, "  Recovery: andamio course student submit --course-id %s --module-code %s --evidence-file <path>\n", courseID, moduleCode)
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
			SltHash:       sltHash,
			OffchainError: offchainError,
		}
		return output.PrintJSON(commitResult)
	}

	return nil
}

