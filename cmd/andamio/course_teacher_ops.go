package main

import (
	"fmt"
	"os"
	"strings"

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
	courseTeacherUpdateModuleStatusCmd.Flags().String("slt-hash", "", "SLT hash (required when status is APPROVED)")

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

// runCourseTeacherRegisterModule handles register-module. When the gateway rejects
// the call because the module already exists (created earlier by `course import --create`
// or a prior partial run), this handler inspects the existing module and either:
//   - advances DRAFT → APPROVED via update-status (matching slt_hash), or
//   - exits 0 as a no-op when already APPROVED/PENDING_TX/ON_CHAIN with matching hash, or
//   - returns an error wrapping the original gateway error when the supplied hash mismatches.
//
// The JSON envelope is stable across all branches:
//
//	{action, status, slt_hash, advanced_from}
//
// where action ∈ {"registered", "advanced", "already_registered"} and advanced_from is
// "DRAFT" only on the advance branch (null otherwise).
func runCourseTeacherRegisterModule(cmd *cobra.Command, args []string) error {
	courseID, _ := cmd.Flags().GetString("course-id")
	moduleCode, _ := cmd.Flags().GetString("module-code")
	sltHash, _ := cmd.Flags().GetString("slt-hash")
	isJSON := output.GetFormat() == output.FormatJSON

	var err error
	sltHash, err = normalizeSltHashFlag(sltHash)
	if err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	c := client.New(cfg)

	if !isJSON {
		fmt.Fprintf(os.Stderr, "Registering module %s...\n", moduleCode)
	}

	resp, err := postRegisterModule(c, courseID, moduleCode, sltHash)
	if err == nil {
		// Happy path — gateway accepted the registration.
		envelope := map[string]interface{}{
			"action":        "registered",
			"status":        "APPROVED",
			"slt_hash":      sltHash,
			"advanced_from": nil,
			"response":      resp,
		}
		if isJSON {
			return output.PrintJSON(envelope)
		}
		fmt.Fprintf(os.Stderr, "Module %s: registered.\n", moduleCode)
		return nil
	}

	if !isModuleAlreadyExistsError(err) {
		return fmt.Errorf("failed to register module: %w", err)
	}

	// Conflict path: look up the existing module to decide what to do.
	existing, lookupErr := lookupTeacherModule(c, courseID, moduleCode)
	if lookupErr != nil {
		return fmt.Errorf("module %s already exists, but could not locate it for recovery: %w (original error: %v)", moduleCode, lookupErr, err)
	}

	existingHash := existing.SltHash
	existingStatus := existing.Status

	if existingHash != sltHash {
		return mismatchError(courseID, moduleCode, existingHash, sltHash, err)
	}

	// Hashes match — branch on current status.
	switch existingStatus {
	case "DRAFT":
		updateResp, updateErr := postUpdateModuleStatus(c, courseID, moduleCode, "APPROVED", sltHash)
		if updateErr != nil {
			return fmt.Errorf("module %s exists in DRAFT with matching hash, but advancing to APPROVED failed: %w", moduleCode, updateErr)
		}
		envelope := map[string]interface{}{
			"action":        "advanced",
			"status":        "APPROVED",
			"slt_hash":      sltHash,
			"advanced_from": "DRAFT",
			"response":      updateResp,
		}
		if isJSON {
			return output.PrintJSON(envelope)
		}
		fmt.Fprintf(os.Stderr, "Module %s: advanced from DRAFT to APPROVED.\n", moduleCode)
		return nil

	case "APPROVED", "PENDING_TX", "ON_CHAIN":
		envelope := map[string]interface{}{
			"action":        "already_registered",
			"status":        existingStatus,
			"slt_hash":      sltHash,
			"advanced_from": nil,
			"response":      nil,
		}
		if isJSON {
			return output.PrintJSON(envelope)
		}
		fmt.Fprintf(os.Stderr, "Module %s: already registered (status: %s).\n", moduleCode, existingStatus)
		return nil

	default:
		return fmt.Errorf("module %s exists in unexpected status %q with matching hash; not advancing automatically", moduleCode, existingStatus)
	}
}

// postRegisterModule performs the bare gateway POST for course-module/register.
// Returns the raw response on success or the wrapped error on failure.
func postRegisterModule(c *client.Client, courseID, moduleCode, sltHash string) (map[string]interface{}, error) {
	payload := map[string]interface{}{
		"course_id":          courseID,
		"course_module_code": moduleCode,
		"slt_hash":           sltHash,
	}
	var resp map[string]interface{}
	if err := c.Post("/api/v2/course/teacher/course-module/register", payload, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// isModuleAlreadyExistsError reports whether err looks like a duplicate course-module
// conflict from the gateway. Requires both an "already exists" stem AND the specific
// "course_module_code" token, so unrelated conflicts (duplicate teacher, duplicate
// credential, "asset module already exists", "module github.com/...: already exists"
// in proxied 5xx bodies) do not route into the module-lookup recovery branch.
//
// TODO: Replace with a typed ConflictError in internal/client once that refactor lands.
func isModuleAlreadyExistsError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "already exists") && strings.Contains(msg, "course_module_code")
}

// existingModule is the minimal projection of a teacher-list module record used by
// the conflict recovery path. Code is omitted because the caller already has it.
type existingModule struct {
	SltHash string
	Status  string
}

// normalizeSltHashFlag trims surrounding whitespace from a user-supplied --slt-hash
// value and rejects empty input. cobra.MarkFlagRequired only enforces that the flag
// was set, not that its value is non-empty; an empty (or whitespace-only) hash could
// silently match an on-chain module whose slt_hash field is null/empty, and a padded
// hash would defeat the byte-exact mismatch comparison.
func normalizeSltHashFlag(sltHash string) (string, error) {
	trimmed := strings.TrimSpace(sltHash)
	if trimmed == "" {
		return "", fmt.Errorf("--slt-hash must be non-empty")
	}
	return trimmed, nil
}

// mismatchError formats the user-facing error returned when register-module hits a
// conflict and the existing module's slt_hash does not match what the caller supplied.
// The original gateway error is wrapped via %w so consumers can use errors.Unwrap.
// The remediation command is on its own line so a copy-to-end-of-line selection does
// not pick up the wrapped error text.
func mismatchError(courseID, moduleCode, existingHash, suppliedHash string, gatewayErr error) error {
	return fmt.Errorf(
		"module %s already exists with slt_hash %s (you supplied %s) [original gateway error: %w]. To replace, run:\n  andamio course teacher delete-module --course-id %s --module-code %s",
		moduleCode, existingHash, suppliedHash, gatewayErr, courseID, moduleCode,
	)
}

// lookupTeacherModule fetches the teacher modules list and returns the entry matching
// moduleCode. The teacher list endpoint returns both draft and on-chain modules with
// their content nested under a "content" object (mirrors course_export.go usage).
//
// Field lookups are defensive: slt_hash and status may appear at the top level, under
// "content", or under either of two field-name conventions across environments.
func lookupTeacherModule(c *client.Client, courseID, moduleCode string) (*existingModule, error) {
	var resp map[string]interface{}
	if err := c.Post("/api/v2/course/teacher/course-modules/list", map[string]string{"course_id": courseID}, &resp); err != nil {
		return nil, fmt.Errorf("failed to list modules for course %s: %w", courseID, err)
	}
	data, ok := resp["data"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected response format from teacher modules list: missing data array")
	}
	for _, item := range data {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		code := lookupStringField(m, "course_module_code")
		if code != moduleCode {
			continue
		}
		return &existingModule{
			SltHash: lookupStringField(m, "slt_hash", "course_module_slt_hash"),
			Status:  lookupStringField(m, "course_module_status", "module_status", "status"),
		}, nil
	}
	return nil, fmt.Errorf("module %s not found in teacher list for course %s", moduleCode, courseID)
}

// lookupStringField returns the first non-empty string match for any of the given
// field names, checking the top level of m first and then m["content"]. Used for
// defensive lookup when API field names vary across environments or response shapes.
func lookupStringField(m map[string]interface{}, names ...string) string {
	for _, name := range names {
		if v, ok := m[name].(string); ok && v != "" {
			return v
		}
	}
	if content, ok := m["content"].(map[string]interface{}); ok {
		for _, name := range names {
			if v, ok := content[name].(string); ok && v != "" {
				return v
			}
		}
	}
	return ""
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
	sltHash, _ := cmd.Flags().GetString("slt-hash")
	isJSON := output.GetFormat() == output.FormatJSON

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	if !isJSON {
		fmt.Fprintf(os.Stderr, "Updating module %s status to %s...\n", moduleCode, status)
	}

	c := client.New(cfg)
	resp, err := postUpdateModuleStatus(c, courseID, moduleCode, status, sltHash)
	if err != nil {
		return fmt.Errorf("failed to update module status: %w", err)
	}

	if isJSON {
		return output.PrintJSON(resp)
	}

	fmt.Fprintf(os.Stderr, "Module %s status updated to %s.\n", moduleCode, status)
	return nil
}

// postUpdateModuleStatus performs the bare gateway POST for course-module/update-status
// and returns the raw response. Callers are responsible for all stdout/stderr output;
// the helper does no printing so it can be composed by both update-module-status and
// register-module's recovery branch.
func postUpdateModuleStatus(c *client.Client, courseID, moduleCode, status, sltHash string) (map[string]interface{}, error) {
	payload := map[string]interface{}{
		"course_id":          courseID,
		"course_module_code": moduleCode,
		"status":             status,
	}
	if sltHash != "" {
		payload["slt_hash"] = sltHash
	}
	var resp map[string]interface{}
	if err := c.Post("/api/v2/course/teacher/course-module/update-status", payload, &resp); err != nil {
		return nil, err
	}
	return resp, nil
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
