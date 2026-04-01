package main

import (
	"fmt"
	"os"

	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/Andamio-Platform/andamio-cli/internal/config"
	"github.com/Andamio-Platform/andamio-cli/internal/output"
	"github.com/spf13/cobra"
)

// courseTeacherCmd is the nested "course teacher" subgroup for module lifecycle and reviews.
// The existing top-level "teacher" command stays as-is — both work.
var courseTeacherCmd = &cobra.Command{
	Use:               "teacher",
	Short:             "Course teacher operations (requires user login)",
	PersistentPreRunE: jwtAuthPreRunE,
}

var courseTeacherRegisterModuleCmd = &cobra.Command{
	Use:   "register-module",
	Short: "Register a course module from on-chain data",
	RunE:  runCourseTeacherRegisterModule,
}

var courseTeacherPublishModuleCmd = &cobra.Command{
	Use:   "publish-module",
	Short: "Publish a course module",
	RunE:  runCourseTeacherPublishModule,
}

var courseTeacherDeleteModuleCmd = &cobra.Command{
	Use:   "delete-module",
	Short: "Delete a course module",
	RunE:  runCourseTeacherModuleAction("/api/v2/course/teacher/course-module/delete", "Deleting"),
}

var courseTeacherUpdateModuleStatusCmd = &cobra.Command{
	Use:   "update-module-status",
	Short: "Update a course module's status",
	Long: `Update the status of a course module.

Examples:
  andamio course teacher update-module-status --course-id <id> --module-code 101 --status DRAFT`,
	RunE: runCourseTeacherUpdateModuleStatus,
}

var courseTeacherReviewCmd = &cobra.Command{
	Use:   "review",
	Short: "Review a student assignment commitment",
	Long: `Review a student's assignment submission. Accept or refuse.

Examples:
  andamio course teacher review --course-id <id> --module-code 101 --participant-alias student1 --decision accept
  andamio course teacher review --course-id <id> --module-code 101 --participant-alias student1 --decision refuse`,
	RunE: runCourseTeacherReview,
}

var courseTeacherCommitmentsCmd = &cobra.Command{
	Use:   "commitments",
	Short: "List pending assignment reviews",
	Long: `List assignment commitments awaiting review for a course.

Examples:
  andamio course teacher commitments --course-id <id>`,
	RunE: runCourseTeacherCommitments,
}

func init() {
	courseCmd.AddCommand(courseTeacherCmd)
	courseTeacherCmd.AddCommand(courseTeacherRegisterModuleCmd)
	courseTeacherCmd.AddCommand(courseTeacherPublishModuleCmd)
	courseTeacherCmd.AddCommand(courseTeacherDeleteModuleCmd)
	courseTeacherCmd.AddCommand(courseTeacherUpdateModuleStatusCmd)
	courseTeacherCmd.AddCommand(courseTeacherReviewCmd)
	courseTeacherCmd.AddCommand(courseTeacherCommitmentsCmd)

	// register-module has an extra required flag (slt-hash)
	courseTeacherRegisterModuleCmd.Flags().String("course-id", "", "Course ID (required)")
	courseTeacherRegisterModuleCmd.MarkFlagRequired("course-id")
	courseTeacherRegisterModuleCmd.Flags().String("module-code", "", "Module code (required)")
	courseTeacherRegisterModuleCmd.MarkFlagRequired("module-code")
	courseTeacherRegisterModuleCmd.Flags().String("slt-hash", "", "SLT hash — on-chain module identifier (required)")
	courseTeacherRegisterModuleCmd.MarkFlagRequired("slt-hash")

	// Module action flags (shared across publish, delete)
	for _, cmd := range []*cobra.Command{
		courseTeacherPublishModuleCmd,
		courseTeacherDeleteModuleCmd,
	} {
		cmd.Flags().String("course-id", "", "Course ID (required)")
		cmd.MarkFlagRequired("course-id")
		cmd.Flags().String("module-code", "", "Module code (required)")
		cmd.MarkFlagRequired("module-code")
	}

	// update-module-status flags
	courseTeacherUpdateModuleStatusCmd.Flags().String("course-id", "", "Course ID (required)")
	courseTeacherUpdateModuleStatusCmd.MarkFlagRequired("course-id")
	courseTeacherUpdateModuleStatusCmd.Flags().String("module-code", "", "Module code (required)")
	courseTeacherUpdateModuleStatusCmd.MarkFlagRequired("module-code")
	courseTeacherUpdateModuleStatusCmd.Flags().String("status", "", "New status (required)")
	courseTeacherUpdateModuleStatusCmd.MarkFlagRequired("status")

	// review flags
	courseTeacherReviewCmd.Flags().String("course-id", "", "Course ID (required)")
	courseTeacherReviewCmd.MarkFlagRequired("course-id")
	courseTeacherReviewCmd.Flags().String("module-code", "", "Module code (required)")
	courseTeacherReviewCmd.MarkFlagRequired("module-code")
	courseTeacherReviewCmd.Flags().String("participant-alias", "", "Student alias (required)")
	courseTeacherReviewCmd.MarkFlagRequired("participant-alias")
	courseTeacherReviewCmd.Flags().String("decision", "", "Review decision: accept or refuse (required)")
	courseTeacherReviewCmd.MarkFlagRequired("decision")

	// commitments flags
	courseTeacherCommitmentsCmd.Flags().String("course-id", "", "Course ID (required)")
	courseTeacherCommitmentsCmd.MarkFlagRequired("course-id")
}

// runCourseTeacherRegisterModule handles register-module which requires slt_hash in addition
// to course-id and module-code.
func runCourseTeacherRegisterModule(cmd *cobra.Command, args []string) error {
	courseID, _ := cmd.Flags().GetString("course-id")
	moduleCode, _ := cmd.Flags().GetString("module-code")
	sltHash, _ := cmd.Flags().GetString("slt-hash")
	isJSON := output.GetFormat() == output.FormatJSON

	payload := map[string]interface{}{
		"course_id":          courseID,
		"course_module_code": moduleCode,
		"slt_hash":           sltHash,
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	if !isJSON {
		fmt.Fprintf(os.Stderr, "Registering module %s...\n", moduleCode)
	}

	c := client.New(cfg)
	var resp map[string]interface{}
	if err := c.Post("/api/v2/course/teacher/course-module/register", payload, &resp); err != nil {
		return fmt.Errorf("failed to register module: %w", err)
	}

	if isJSON {
		return output.PrintJSON(resp)
	}

	fmt.Fprintf(os.Stderr, "Module %s: registered.\n", moduleCode)
	return nil
}

// runCourseTeacherPublishModule is a dedicated handler for publish-module that inspects
// the API response for signals that the module was actually linked to an on-chain counterpart.
// Unlike the generic handler, it warns when the publish appears to be a no-op.
func runCourseTeacherPublishModule(cmd *cobra.Command, args []string) error {
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
		fmt.Fprintf(os.Stderr, "Publishing module %s...\n", moduleCode)
	}

	c := client.New(cfg)
	var resp map[string]interface{}
	if err := c.Post("/api/v2/course/teacher/course-module/publish", payload, &resp); err != nil {
		return fmt.Errorf("failed to publish module: %w", err)
	}

	// Inspect response for linkage signals
	source, hasSource := resp["source"]
	warningMsg, hasWarning := resp["warning"]
	linked := hasSource && source == "merged"

	if hasWarning {
		fmt.Fprintf(os.Stderr, "Warning: %v\n", warningMsg)
	}

	if !linked {
		fmt.Fprintf(os.Stderr, "Warning: module %s may not have been linked to an on-chain module.\n"+
			"Ensure the module exists on-chain first (use 'andamio tx run' with modules_manage).\n"+
			"Then link with: andamio course teacher register-module --course-id %s --module-code %s --slt-hash <hash>\n",
			moduleCode, courseID, moduleCode)
	}

	if isJSON {
		return output.PrintJSON(resp)
	}

	if linked {
		fmt.Fprintf(os.Stderr, "Module %s: published.\n", moduleCode)
	} else {
		fmt.Fprintf(os.Stderr, "Module %s: done.\n", moduleCode)
	}
	return nil
}

// runCourseTeacherModuleAction returns a RunE function for module lifecycle commands
// that take course-id and module-code. Used by delete-module.
func runCourseTeacherModuleAction(endpoint, verb string) func(cmd *cobra.Command, args []string) error {
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
			fmt.Fprintf(os.Stderr, "%s module %s...\n", verb, moduleCode)
		}

		c := client.New(cfg)
		var resp map[string]interface{}
		if err := c.Post(endpoint, payload, &resp); err != nil {
			return fmt.Errorf("failed to %s module: %w", verb, err)
		}

		if isJSON {
			return output.PrintJSON(resp)
		}

		fmt.Fprintf(os.Stderr, "Module %s: done.\n", moduleCode)
		return nil
	}
}

func runCourseTeacherUpdateModuleStatus(cmd *cobra.Command, args []string) error {
	courseID, _ := cmd.Flags().GetString("course-id")
	moduleCode, _ := cmd.Flags().GetString("module-code")
	status, _ := cmd.Flags().GetString("status")
	isJSON := output.GetFormat() == output.FormatJSON

	payload := map[string]interface{}{
		"course_id":          courseID,
		"course_module_code": moduleCode,
		"status":             status,
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	if !isJSON {
		fmt.Fprintf(os.Stderr, "Updating module %s status to %s...\n", moduleCode, status)
	}

	c := client.New(cfg)
	var resp map[string]interface{}
	if err := c.Post("/api/v2/course/teacher/course-module/update-status", payload, &resp); err != nil {
		return fmt.Errorf("failed to update module status: %w", err)
	}

	if isJSON {
		return output.PrintJSON(resp)
	}

	fmt.Fprintf(os.Stderr, "Module %s status updated to %s.\n", moduleCode, status)
	return nil
}

func runCourseTeacherReview(cmd *cobra.Command, args []string) error {
	courseID, _ := cmd.Flags().GetString("course-id")
	moduleCode, _ := cmd.Flags().GetString("module-code")
	participantAlias, _ := cmd.Flags().GetString("participant-alias")
	decision, _ := cmd.Flags().GetString("decision")
	isJSON := output.GetFormat() == output.FormatJSON

	if decision != "accept" && decision != "refuse" {
		return fmt.Errorf("--decision must be 'accept' or 'refuse', got %q", decision)
	}

	payload := map[string]interface{}{
		"course_id":          courseID,
		"course_module_code": moduleCode,
		"participant_alias":  participantAlias,
		"decision":           decision,
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	if !isJSON {
		fmt.Fprintf(os.Stderr, "Reviewing %s (module %s): %s\n", participantAlias, moduleCode, decision)
	}

	c := client.New(cfg)
	var resp map[string]interface{}
	if err := c.Post("/api/v2/course/teacher/assignment-commitment/review", payload, &resp); err != nil {
		return fmt.Errorf("failed to review commitment: %w", err)
	}

	if isJSON {
		return output.PrintJSON(resp)
	}

	fmt.Fprintf(os.Stderr, "Commitment reviewed: %s.\n", decision)
	return nil
}

func runCourseTeacherCommitments(cmd *cobra.Command, args []string) error {
	courseID, _ := cmd.Flags().GetString("course-id")
	return printListPost(
		"/api/v2/course/teacher/assignment-commitments/list",
		map[string]string{"course_id": courseID},
		"No pending reviews found.",
		"content.title", "commitment_id",
	)
}
