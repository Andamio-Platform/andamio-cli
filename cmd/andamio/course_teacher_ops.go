package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Andamio-Platform/andamio-cli/internal/apierr"
	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/Andamio-Platform/andamio-cli/internal/config"
	"github.com/Andamio-Platform/andamio-cli/internal/output"
	"github.com/spf13/cobra"
)

// RegisterModuleEnvelope is the stable --output json contract for
// `course teacher register-module`. All success branches emit this shape;
// errors render via the global {"error": "..."} path in main.go.
//
// Action takes one of three values, each with different nullability:
//
//   - "registered"         — the module was newly created on this call. Response
//                            wraps the gateway register-response body. AdvancedFrom is nil.
//   - "advanced"           — an existing DRAFT module with matching slt_hash was
//                            advanced to APPROVED on this call. Response wraps the
//                            update-status response. AdvancedFrom is non-nil (today
//                            always "DRAFT", the only status we advance from).
//   - "already_registered" — an existing module with matching slt_hash was found
//                            in a non-DRAFT state; this call was a no-op. Response
//                            is nil (no gateway round-trip happened after the 409).
//                            AdvancedFrom is nil.
//
// Status and SltHash are populated from the gateway response via
// lookupStringField's defensive key search, with hardcoded fallbacks when the
// response is minimal. SltHash canonicalization is asymmetric by branch
// (todo #021 unblocks full symmetry):
//   - "already_registered": always canonical (existing.SltHash from the teacher
//     modules list). Supplied "ABC123" against stored "abc123" returns "abc123".
//   - "registered" / "advanced": gateway field if present, else the supplied
//     hash (post-trim). Today's gateway typically doesn't populate it on these
//     branches, so consumers see supplied casing until real preprod fixtures land.
//
// Scripts that need guaranteed canonical values should treat this envelope as
// transactional and consult `course modules --output json` for authoritative
// hashes. Within the envelope: blake2b hex is case-insensitive by convention;
// use strings.EqualFold or lowercase-normalize both sides when comparing.
// Branch on Action; `--output json` is the stable automation surface.
type RegisterModuleEnvelope struct {
	Action       string         `json:"action"`
	Status       string         `json:"status"`
	SltHash      string         `json:"slt_hash"`
	AdvancedFrom *string        `json:"advanced_from"`
	Response     map[string]any `json:"response"`
}

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
	Long: `Register a course module from on-chain data.

Idempotent on slt_hash match. When the module already exists in the DB
(typically because 'course import --create' created it, or a prior run
partially completed), behavior depends on its current status:

  DRAFT          + matching hash -> advances to APPROVED
  APPROVED       + matching hash -> no-op (exit 0)
  PENDING_TX     + matching hash -> no-op (exit 0)
  ON_CHAIN       + matching hash -> no-op (exit 0)
  hash mismatch  (any status)    -> error; suggests delete-module

With --output json, success branches emit RegisterModuleEnvelope (see
the Go struct for the authoritative field-level contract). Scripts
should branch on 'action' — text mode is for humans, --output json is
the stable surface for automation. Error branches return the global
{"error": "..."} shape, not the envelope.

Examples:
  andamio course teacher register-module --course-id <id> --module-code 101 --slt-hash <hash>
  andamio course teacher register-module --course-id <id> --module-code 101 --slt-hash <hash> --output json`,
	RunE: runCourseTeacherRegisterModule,
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

// runCourseTeacherRegisterModule handles register-module. See
// registerOrRecoverModule for the recovery flow and RegisterModuleEnvelope for
// the --output json shape. Error branches (mismatch, lookup failure,
// unexpected status) return a Go error rendered via the global
// {"error": "..."} path in main.go, not the envelope.
func runCourseTeacherRegisterModule(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	courseID, _ := cmd.Flags().GetString("course-id")
	moduleCode, _ := cmd.Flags().GetString("module-code")
	sltHash, _ := cmd.Flags().GetString("slt-hash")
	isJSON := output.GetFormat() == output.FormatJSON

	sltHash, err := normalizeSltHashFlag(sltHash)
	if err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	c := client.New(cfg)

	// Wire a retry log to stderr for the recovery list call. Suppressed in
	// JSON mode to keep the scripting surface clean. lookupTeacherModule is
	// the only PostWithRetry caller today; when more land, factor this into
	// a shared helper.
	if !isJSON {
		c.SetOnRetry(func(attempt int, wait time.Duration, err error) {
			fmt.Fprintf(os.Stderr, "  retrying in %s (attempt %d): %v\n", wait.Round(time.Millisecond), attempt, err)
		})
	}

	if !isJSON {
		fmt.Fprintf(os.Stderr, "Registering module %s...\n", moduleCode)
	}

	envelope, successMsg, err := registerOrRecoverModule(ctx, c, courseID, moduleCode, sltHash, isJSON)
	if err != nil {
		return err
	}

	if isJSON {
		return output.PrintJSON(envelope)
	}
	fmt.Fprintln(os.Stderr, successMsg)
	return nil
}

// registerOrRecoverModule drives the register-module POST and, on an "already exists"
// conflict, performs the hash-compare / status-branch recovery. See
// RegisterModuleEnvelope for the --output json shape.
//
// isJSON controls only the single intermediate progress line printed to stderr when a
// conflict is detected (so humans see the recovery is in flight rather than staring at
// a silent terminal for up to two 30 s POSTs). Tests pass true to keep stderr quiet.
func registerOrRecoverModule(ctx context.Context, c *client.Client, courseID, moduleCode, sltHash string, isJSON bool) (*RegisterModuleEnvelope, string, error) {
	resp, err := postRegisterModule(ctx, c, courseID, moduleCode, sltHash)
	if err == nil {
		// Gateway-state population: prefer real response fields when the gateway
		// supplies them. Candidate key names follow lookupTeacherModule's drift
		// handling (slt_hash / course_module_slt_hash, status / course_module_status).
		// When the response is minimal (today's observed behavior — see plan's
		// Key Technical Decisions), the fallbacks preserve the pre-refactor contract.
		status := lookupStringField(resp, "status", "course_module_status", "module_status")
		if status == "" {
			status = "APPROVED"
		}
		respHash := lookupStringField(resp, "slt_hash", "course_module_slt_hash")
		if respHash == "" {
			respHash = sltHash
		}
		return &RegisterModuleEnvelope{
			Action:       "registered",
			Status:       status,
			SltHash:      respHash,
			AdvancedFrom: nil,
			Response:     resp,
		}, fmt.Sprintf("Module %s: registered.", moduleCode), nil
	}

	if !isModuleAlreadyExistsError(err) {
		return nil, "", fmt.Errorf("failed to register module: %w", err)
	}

	// Recovery is about to do another gateway round-trip. Tell the user so a slow
	// list call doesn't look like a hang and prompt an ill-timed ctrl-C.
	if !isJSON {
		fmt.Fprintf(os.Stderr, "Module %s: already exists in DB, checking status...\n", moduleCode)
	}

	existing, lookupErr := lookupTeacherModule(ctx, c, courseID, moduleCode)
	if lookupErr != nil {
		return nil, "", fmt.Errorf("module %s already exists, but could not locate it for recovery: %w (original error: %v)", moduleCode, lookupErr, err)
	}

	// Case-insensitive compare: blake2b hex hashes are case-insensitive by convention and
	// users commonly paste them from explorers that may use either case. A case-only
	// difference would otherwise route to the destructive delete-module remediation.
	if !strings.EqualFold(existing.SltHash, sltHash) {
		return nil, "", mismatchError(courseID, moduleCode, existing.SltHash, sltHash, err)
	}

	switch existing.Status {
	case "DRAFT":
		updateResp, updateErr := postUpdateModuleStatus(ctx, c, courseID, moduleCode, "APPROVED", sltHash)
		if updateErr != nil {
			return nil, "", fmt.Errorf("module %s exists in DRAFT with matching hash, but advancing to APPROVED failed: %w", moduleCode, updateErr)
		}
		// Same gateway-state-first pattern as the registered branch; today's
		// update-status response shape typically lacks these fields, so the
		// supplied/hardcoded fallbacks carry the envelope. Widen the key
		// candidates when preprod fixtures capture a real response.
		status := lookupStringField(updateResp, "status", "course_module_status", "module_status")
		if status == "" {
			status = "APPROVED"
		}
		respHash := lookupStringField(updateResp, "slt_hash", "course_module_slt_hash")
		if respHash == "" {
			respHash = sltHash
		}
		draftFrom := "DRAFT"
		return &RegisterModuleEnvelope{
			Action:       "advanced",
			Status:       status,
			SltHash:      respHash,
			AdvancedFrom: &draftFrom,
			Response:     updateResp,
		}, fmt.Sprintf("Module %s: advanced from DRAFT to APPROVED.", moduleCode), nil

	case "APPROVED", "PENDING_TX", "ON_CHAIN":
		// Canonical values from the teacher-list lookup. No additional gateway
		// round-trip happened on this branch — Response is nil. This is the only
		// branch where SltHash is guaranteed canonical today (survives the
		// supplied-uppercase / stored-lowercase case-insensitivity surface).
		return &RegisterModuleEnvelope{
			Action:       "already_registered",
			Status:       existing.Status,
			SltHash:      existing.SltHash,
			AdvancedFrom: nil,
			Response:     nil,
		}, fmt.Sprintf("Module %s: already registered (status: %s).", moduleCode, existing.Status), nil

	default:
		return nil, "", fmt.Errorf("module %s exists in unexpected status %q with matching hash; not advancing automatically", moduleCode, existing.Status)
	}
}

// postRegisterModule performs the bare gateway POST for course-module/register.
// Returns the raw response on success or the wrapped error on failure. Plain
// (non-retrying) POST: this is a mutating call and the caller's recovery logic
// depends on observing a 409 conflict on the first attempt.
func postRegisterModule(ctx context.Context, c *client.Client, courseID, moduleCode, sltHash string) (map[string]interface{}, error) {
	payload := map[string]interface{}{
		"course_id":          courseID,
		"course_module_code": moduleCode,
		"slt_hash":           sltHash,
	}
	var resp map[string]interface{}
	if err := c.Post(ctx, "/api/v2/course/teacher/course-module/register", payload, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// isModuleAlreadyExistsError reports whether err looks like a duplicate course-module
// conflict from the gateway. Three gates, ANDed together:
//   - errors.As against *apierr.ConflictError gates on HTTP 409 (surfaced by internal/client)
//   - "already exists" body substring gates on conflict semantics (vs e.g. 409 validation)
//   - "course_module_code" body substring gates on the specific field (vs other 409s like
//     duplicate teacher, duplicate credential, etc.)
func isModuleAlreadyExistsError(err error) bool {
	if err == nil {
		return false
	}
	var conflict *apierr.ConflictError
	if !errors.As(err, &conflict) {
		return false
	}
	msg := strings.ToLower(conflict.Message)
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
// Multi-line layout so the two hashes and the remediation don't get lost in a ~200-char
// single-line blob at stderr width.
func mismatchError(courseID, moduleCode, existingHash, suppliedHash string, gatewayErr error) error {
	return fmt.Errorf(
		"module %s already exists with slt_hash mismatch:\n  stored:   %s\n  supplied: %s\n  gateway:  %w\nTo replace, run:\n  andamio course teacher delete-module --course-id %s --module-code %s",
		moduleCode, existingHash, suppliedHash, gatewayErr, courseID, moduleCode,
	)
}

// lookupTeacherModule fetches the teacher modules list and returns the entry matching
// moduleCode. The teacher list endpoint returns both draft and on-chain modules with
// their content nested under a "content" object (mirrors course_export.go usage).
//
// Field lookups are defensive: slt_hash and status may appear at the top level, under
// "content", or under either of two field-name conventions across environments.
func lookupTeacherModule(ctx context.Context, c *client.Client, courseID, moduleCode string) (*existingModule, error) {
	var resp map[string]interface{}
	// List POST is read-only (no side effects) despite the verb, so retry
	// transient 5xx/network blips. The caller (registerOrRecoverModule) has
	// already observed a 409 conflict and cannot make progress without this
	// lookup, so a single transient error should not be fatal.
	if err := c.PostWithRetry(ctx, "/api/v2/course/teacher/course-modules/list", map[string]string{"course_id": courseID}, &resp); err != nil {
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
		sltHash := lookupStringField(m, "slt_hash", "course_module_slt_hash")
		if sltHash == "" {
			// Surface response-shape drift explicitly rather than letting an empty hash
			// cascade into a mismatch error that points the user at destructive delete-module.
			return nil, fmt.Errorf("module %s found in teacher list but slt_hash field is missing or empty (response shape may have changed)", moduleCode)
		}
		status := lookupStringField(m, "course_module_status", "module_status", "status")
		if status == "" {
			return nil, fmt.Errorf("module %s found in teacher list but status field is missing or empty (response shape may have changed)", moduleCode)
		}
		return &existingModule{
			SltHash: sltHash,
			Status:  status,
		}, nil
	}
	return nil, fmt.Errorf("module %s not found in teacher list for course %s", moduleCode, courseID)
}

// lookupStringField returns the first non-empty string match for any of the given
// field names, checking the top level of m first and then m["content"]. Values
// are whitespace-trimmed before the non-empty check and before returning — a
// gateway response with "status": " " or "status": "\t\n" is treated as
// "not present" so the caller's fallback kicks in, and a surrounding-whitespace
// value like " APPROVED " returns as "APPROVED" rather than leaking padding
// into downstream equality checks against canonical status strings.
// Used for defensive lookup when API field names vary across environments or response shapes.
func lookupStringField(m map[string]interface{}, names ...string) string {
	for _, name := range names {
		if v, ok := m[name].(string); ok {
			if trimmed := strings.TrimSpace(v); trimmed != "" {
				return trimmed
			}
		}
	}
	if content, ok := m["content"].(map[string]interface{}); ok {
		for _, name := range names {
			if v, ok := content[name].(string); ok {
				if trimmed := strings.TrimSpace(v); trimmed != "" {
					return trimmed
				}
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
	if err := c.Post(cmd.Context(), "/api/v2/course/teacher/course-module/publish", payload, &resp); err != nil {
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
		if err := c.Post(cmd.Context(), endpoint, payload, &resp); err != nil {
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
	resp, err := postUpdateModuleStatus(cmd.Context(), c, courseID, moduleCode, status, sltHash)
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
func postUpdateModuleStatus(ctx context.Context, c *client.Client, courseID, moduleCode, status, sltHash string) (map[string]interface{}, error) {
	payload := map[string]interface{}{
		"course_id":          courseID,
		"course_module_code": moduleCode,
		"status":             status,
	}
	if sltHash != "" {
		payload["slt_hash"] = sltHash
	}
	var resp map[string]interface{}
	if err := c.Post(ctx, "/api/v2/course/teacher/course-module/update-status", payload, &resp); err != nil {
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
	if err := c.Post(cmd.Context(), "/api/v2/course/teacher/assignment-commitment/review", payload, &resp); err != nil {
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
		cmd.Context(),
		"/api/v2/course/teacher/assignment-commitments/list",
		map[string]string{"course_id": courseID},
		"No pending reviews found.",
		"content.title", "commitment_id",
	)
}
