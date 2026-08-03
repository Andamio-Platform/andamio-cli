package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
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

Machine-readable output contract (--output json):

  .data[]                         one row per commitment
  .data[].student_alias           on-chain alias of the submitter
  .data[].course_module_code      module the commitment belongs to
  .data[].course_id               course the commitment belongs to
  .data[].content                 present with --course; absent on the
                                  no-filter summary
  .data[].content.commitment_status  raw gateway enum (see above)
  .data[].content.evidence        the submission as a Tiptap JSON document,
                                  passed through verbatim — this is the
                                  hash-bearing form
  .data[].content.evidence_text   the same submission rendered as Markdown,
                                  added by the CLI. Absent when there is no
                                  evidence. Read this to get the prose; read
                                  .content.evidence to verify a hash.

Read a submission without walking the Tiptap tree:
  andamio teacher assignments list --course <id> --output json \
    | jq -r '.data[] | select(.content.commitment_status=="SUBMITTED")
             | "\(.student_alias): \(.content.evidence_text)"'

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

Emits the matched row from 'teacher assignments list', including
content.evidence_text — the submission rendered as Markdown alongside the
raw Tiptap document in content.evidence. See 'teacher assignments list --help'
for the full output contract.

Read one submission:
  andamio teacher assignments get <course-id> <module-code> <student-alias> \
    --output json | jq -r '.content.evidence_text'

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
// endpoint and returns the decoded envelope, with each row's evidence decoded
// to Markdown (see enrichCommitmentRows). Split from the Cobra handler so tests
// can drive it with an httptest stub without writing to disk config. The
// envelope is returned as map[string]interface{} so the JSON pass-through path
// in the caller emits the gateway response verbatim (no struct decoding would
// silently drop unknown fields).
func fetchTeacherAssignmentsList(ctx context.Context, c *client.Client, courseID string) (map[string]interface{}, error) {
	var body interface{}
	if courseID != "" {
		body = map[string]string{"course_id": courseID}
	}
	var resp map[string]interface{}
	if err := c.Post(ctx, "/api/v2/course/teacher/assignment-commitments/list", body, &resp); err != nil {
		return nil, err
	}
	enrichCommitmentRows(resp)
	return resp, nil
}

// evidenceTextField is the sibling key carrying the learner's submission as
// Markdown. Named as a constant because it is a documented output contract,
// not an implementation detail — see enrichCommitmentEvidence.
const evidenceTextField = "evidence_text"

// enrichCommitmentRows adds decoded evidence to every row in a
// assignment-commitments envelope. Missing, empty or non-array `data` is a
// no-op: the no-`--course` summary response has a different shape and must
// pass through untouched.
func enrichCommitmentRows(resp map[string]interface{}) {
	data, ok := resp["data"].([]interface{})
	if !ok {
		return
	}
	for _, item := range data {
		if row, ok := item.(map[string]interface{}); ok {
			enrichCommitmentEvidence(row)
		}
	}
}

// enrichCommitmentEvidence sets content.evidence_text to the Markdown rendering
// of content.evidence, leaving content.evidence itself untouched.
//
// Why a sibling field rather than a replacement: content.evidence is
// hash-bearing. The on-chain commitment hash is computed over the normalized
// Tiptap document, so the raw structure has to round-trip byte-for-byte for
// hash verification to mean anything. Rewriting it in place would quietly break
// that. Adding a sibling is also non-breaking for existing --output json
// consumers.
//
// Why decode at all: the submission a teacher needs to read arrives as a Tiptap
// document — nested nodes, marks, attrs. Before 1.0 every consumer walked that
// tree itself; the repo's own assess-assignment skill instructed the agent to
// "extract the text". That is CLI work, and it already existed for course
// export (tiptapToMarkdown), so 1.0 wires it up rather than asking each caller
// to reimplement it (issue #124).
//
// Output contract:
//   - evidence_text is present only when content.evidence is a Tiptap document
//     object. It is absent — not empty-string — otherwise.
//   - Rows without a content object (the no-`--course` summary shape) are
//     untouched, exactly as they already are for commitment_status.
//   - content.evidence is never modified.
//
// The image-URL return from tiptapToMarkdown is discarded: evidence is prose,
// and a caller wanting embedded image references can read them from the raw
// document.
func enrichCommitmentEvidence(row map[string]interface{}) {
	content, ok := row["content"].(map[string]interface{})
	if !ok {
		return
	}
	evidence, ok := content["evidence"].(map[string]interface{})
	if !ok {
		return
	}
	text, _ := tiptapToMarkdown(evidence)
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	content[evidenceTextField] = text
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

	// Fetch full commitment data for this course, then filter by module + student.
	// Routed through fetchTeacherAssignmentsList so `get` and `list` cannot
	// diverge in how they decode evidence.
	resp, err := fetchTeacherAssignmentsList(cmd.Context(), c, courseID)
	if err != nil {
		return err
	}

	// A missing or non-array data field means the course has no commitments to
	// match against — the same outcome as searching the list and finding
	// nothing, and it must classify the same way. This branch previously
	// returned an untyped error and exited 1, so "this course has no
	// commitments" was indistinguishable from a server failure, while the
	// otherwise-identical branch at the bottom of this function exited 2.
	data, ok := resp["data"].([]interface{})
	if !ok {
		return &apierr.NotFoundError{
			Message: fmt.Sprintf("no commitments found for course %s. Run 'andamio teacher assignments list --course %s' to check the course id",
				courseID, courseID),
		}
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
