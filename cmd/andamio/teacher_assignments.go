package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"unicode/utf8"

	"github.com/Andamio-Platform/andamio-cli/internal/apierr"
	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/Andamio-Platform/andamio-cli/internal/config"
	"github.com/Andamio-Platform/andamio-cli/internal/output"
	"github.com/spf13/cobra"
)

// statusEmpty is rendered in the Status column when content.commitment_status
// is absent — typically on rows from the no-course-filter "on-chain-only"
// summary response shape. Not an enum value, not an alias: just a visual
// placeholder so the column has something to occupy the cell.
const statusEmpty = "—"

// statusMinWidth is the floor for the Status column. The widest known DB
// enum string is CREDENTIAL_CLAIMED (18 runes); PENDING_TX_* variants fall
// in the same range. 20 leaves a column of breathing room and keeps short
// result sets aligned. The column expands beyond 20 when a value exceeds it
// so the enum is never truncated.
const statusMinWidth = 20

var teacherAssignmentsCmd = &cobra.Command{
	Use:   "assignments",
	Short: "Manage assignment reviews (teacher role)",
}

var teacherAssignmentsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List pending assignment commitments for review",
	Long: `List assignment commitments pending teacher review.

Without --course, returns a lightweight summary across all courses. The
on-chain-only summary has no nested content.commitment_status field, so
the Status column renders "—" in text mode and the field is absent from
the JSON envelope. To get DB statuses, re-run with --course <id>.

With --course, returns full merged history (on-chain + DB) for that course,
with the Status column populated from content.commitment_status (raw API
enum, displayed verbatim). For scripting, use:
  andamio teacher assignments list --course <id> --output json \
    | jq '.data[].content.commitment_status'

Known commitment_status values: AWAITING_SUBMISSION, SUBMITTED, ACCEPTED,
REFUSED, CREDENTIAL_CLAIMED, LEFT, PENDING_TX_* (transient). The CLI does
not validate or alias — whatever string the gateway returns is what you see.

Examples:
  andamio teacher assignments list
  andamio teacher assignments list --course <course-id>
  andamio teacher assignments list --course <course-id> --output json`,
	RunE: runTeacherAssignmentsList,
}

var teacherAssignmentsGetCmd = &cobra.Command{
	Use:   "get <course-id> <module-code> <student-alias>",
	Short: "Get a specific assignment commitment for review",
	Long: `Get full details for a specific student's assignment commitment.

Examples:
  andamio teacher assignments get <course-id> <module-code> <student-alias>
  andamio teacher assignments get <course-id> <module-code> <student-alias> --output json`,
	Args: cobra.ExactArgs(3),
	RunE: runTeacherAssignmentsGet,
}

func init() {
	teacherCmd.AddCommand(teacherAssignmentsCmd)
	teacherAssignmentsCmd.AddCommand(teacherAssignmentsListCmd)
	teacherAssignmentsCmd.AddCommand(teacherAssignmentsGetCmd)

	// List flags (all optional)
	teacherAssignmentsListCmd.Flags().String("course", "", "Filter by course ID")
}

func runTeacherAssignmentsList(cmd *cobra.Command, args []string) error {
	courseID, _ := cmd.Flags().GetString("course")

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	c := client.New(cfg)

	resp, err := fetchTeacherAssignmentsList(cmd.Context(), c, courseID)
	if err != nil {
		return err
	}

	// Non-text formats: pass through raw API response (handles empty data correctly)
	if output.GetFormat() != output.FormatText {
		return output.PrintJSON(resp)
	}

	data, ok := resp["data"].([]interface{})
	if !ok || len(data) == 0 {
		fmt.Fprintln(os.Stderr, "No pending assignment commitments found.")
		return nil
	}

	return renderTeacherAssignmentsListText(data, os.Stdout)
}

// fetchTeacherAssignmentsList POSTs to the assignment-commitments listing
// endpoint and returns the raw decoded envelope. Split from the Cobra handler
// so tests can drive it with an httptest stub without writing to disk config.
// The envelope is returned as map[string]interface{} so the JSON pass-through
// path in the caller emits the gateway response verbatim (no struct decoding
// would silently drop unknown fields).
func fetchTeacherAssignmentsList(ctx context.Context, c *client.Client, courseID string) (map[string]interface{}, error) {
	var body interface{}
	if courseID != "" {
		body = map[string]string{"course_id": courseID}
	}
	var resp map[string]interface{}
	if err := c.Post(ctx, "/api/v2/course/teacher/assignment-commitments/list", body, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// renderTeacherAssignmentsListText writes the text-mode table for the
// non-empty result set. Split from the Cobra handler so tests can exercise
// the column-rendering logic without httptest, mirroring the pattern in
// renderQualifiedContributorsText (project_manager_ops.go).
//
// Column layout: STUDENT (20) | MODULE (12) | SOURCE (15) | STATUS (dyn, ≥20) | COURSE ID.
// Status is read from content.commitment_status. If absent, the cell renders
// statusEmpty. The API enum is displayed verbatim — no aliasing, no truncation.
func renderTeacherAssignmentsListText(data []interface{}, stdout io.Writer) error {
	// First pass: compute the Status column width from the actual data.
	// The width expands above statusMinWidth if any enum exceeds it, so a
	// future longer value (e.g., a new PENDING_TX_* variant) renders verbatim
	// rather than getting silently sliced.
	statusWidth := statusMinWidth
	for _, item := range data {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		status := commitmentStatusOrEmpty(m)
		if w := utf8.RuneCountInString(status); w > statusWidth {
			statusWidth = w
		}
	}

	fmt.Fprintf(stdout, "%-20s %-12s %-15s %s %s\n",
		"STUDENT", "MODULE", "SOURCE", padRunes("STATUS", statusWidth), "COURSE ID")
	fmt.Fprintf(stdout, "%-20s %-12s %-15s %s %s\n",
		"-------", "------", "------", padRunes("------", statusWidth), "---------")

	for _, item := range data {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		student, _ := m["student_alias"].(string)
		moduleCode, _ := m["course_module_code"].(string)
		source, _ := m["source"].(string)
		cID, _ := m["course_id"].(string)
		status := commitmentStatusOrEmpty(m)

		student = truncateUTF8(student, 20)

		fmt.Fprintf(stdout, "%-20s %-12s %-15s %s %s\n",
			student, moduleCode, source, padRunes(status, statusWidth), cID)
	}

	return nil
}

// commitmentStatusOrEmpty reads content.commitment_status with a safe two-step
// type assertion. Returns statusEmpty when content is missing, not a map, or
// commitment_status is absent/empty/non-string. No fallback to on_chain_status
// or any other field — mixing enums in one column would be aliasing.
func commitmentStatusOrEmpty(m map[string]interface{}) string {
	content, ok := m["content"].(map[string]interface{})
	if !ok {
		return statusEmpty
	}
	status, _ := content["commitment_status"].(string)
	if status == "" {
		return statusEmpty
	}
	return status
}

func runTeacherAssignmentsGet(cmd *cobra.Command, args []string) error {
	courseID, moduleCode, studentAlias := args[0], args[1], args[2]

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	c := client.New(cfg)

	// Fetch full commitment data for this course, then filter by module + student
	body := map[string]string{"course_id": courseID}
	var resp map[string]interface{}
	if err := c.Post(cmd.Context(), "/api/v2/course/teacher/assignment-commitments/list", body, &resp); err != nil {
		return err
	}

	data, ok := resp["data"].([]interface{})
	if !ok {
		return fmt.Errorf("no commitments found for course %s", courseID)
	}

	for _, item := range data {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		mCode, _ := m["course_module_code"].(string)
		alias, _ := m["student_alias"].(string)
		if mCode == moduleCode && alias == studentAlias {
			return output.PrintJSON(m)
		}
	}

	return &apierr.NotFoundError{
		Message: fmt.Sprintf("no commitment found for student %q in module %s. Run 'andamio teacher assignments list --course %s' to see pending commitments",
			studentAlias, moduleCode, courseID),
	}
}
