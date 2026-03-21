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
	RunE:  runCourseTeacherModuleAction("/api/v2/course/teacher/course-module/register", "Registering"),
}

var courseTeacherPublishModuleCmd = &cobra.Command{
	Use:   "publish-module",
	Short: "Publish a course module",
	RunE:  runCourseTeacherModuleAction("/api/v2/course/teacher/course-module/publish", "Publishing"),
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
	Long: `Review a student's assignment submission. Approve or reject with optional feedback.

Examples:
  andamio course teacher review --course-id <id> --commitment-id <cid> --decision approve
  andamio course teacher review --course-id <id> --commitment-id <cid> --decision reject --feedback "Needs more detail"`,
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

	// Module action flags (shared across register, publish, delete)
	for _, cmd := range []*cobra.Command{
		courseTeacherRegisterModuleCmd,
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
	courseTeacherReviewCmd.Flags().String("commitment-id", "", "Commitment ID (required)")
	courseTeacherReviewCmd.MarkFlagRequired("commitment-id")
	courseTeacherReviewCmd.Flags().String("decision", "", "Review decision: approve or reject (required)")
	courseTeacherReviewCmd.MarkFlagRequired("decision")
	courseTeacherReviewCmd.Flags().String("feedback", "", "Optional feedback message")

	// commitments flags
	courseTeacherCommitmentsCmd.Flags().String("course-id", "", "Course ID (required)")
	courseTeacherCommitmentsCmd.MarkFlagRequired("course-id")
}

// runCourseTeacherModuleAction returns a RunE function for module lifecycle commands
// that take course-id and module-code.
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
	commitmentID, _ := cmd.Flags().GetString("commitment-id")
	decision, _ := cmd.Flags().GetString("decision")
	feedback, _ := cmd.Flags().GetString("feedback")
	isJSON := output.GetFormat() == output.FormatJSON

	payload := map[string]interface{}{
		"course_id":     courseID,
		"commitment_id": commitmentID,
		"decision":      decision,
	}
	if feedback != "" {
		payload["feedback"] = feedback
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	if !isJSON {
		fmt.Fprintf(os.Stderr, "Reviewing commitment %s: %s\n", commitmentID, decision)
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
	isJSON := output.GetFormat() == output.FormatJSON

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	c := client.New(cfg)
	payload := map[string]string{"course_id": courseID}
	var resp map[string]interface{}
	if err := c.Post("/api/v2/course/teacher/assignment-commitments/list", payload, &resp); err != nil {
		return fmt.Errorf("failed to list commitments: %w", err)
	}

	if isJSON {
		return output.PrintJSON(resp)
	}

	data, ok := resp["data"].([]interface{})
	if !ok || len(data) == 0 {
		fmt.Fprintln(os.Stderr, "No pending reviews found.")
		return nil
	}

	items := make([]map[string]interface{}, 0, len(data))
	for _, item := range data {
		if m, ok := item.(map[string]interface{}); ok {
			items = append(items, m)
		}
	}

	return output.PrintList(items, "content.title", "commitment_id")
}
