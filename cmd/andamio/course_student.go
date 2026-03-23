package main

import (
	"fmt"
	"os"

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
