package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/Andamio-Platform/andamio-cli/internal/apierr"
	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/Andamio-Platform/andamio-cli/internal/config"
	"github.com/Andamio-Platform/andamio-cli/internal/output"
	"github.com/spf13/cobra"
)

func init() {
	courseCmd.AddCommand(courseImportAssignmentCmd)
	courseImportAssignmentCmd.Flags().String("course", "", "Course name or substring (alternative to the course-id argument)")
	courseImportAssignmentCmd.Flags().String("title", "", "Assignment title (default: the module's existing assignment title; required when the module has no assignment yet)")
	courseImportAssignmentCmd.Flags().String("description", "", "Assignment description (default: the existing description)")
	courseImportAssignmentCmd.Flags().Bool("dry-run", false, "Validate and print the summary without sending anything")
	courseImportAssignmentCmd.Flags().Bool("show-payload", false, "With --dry-run, also print the full API payload on stderr")
}

var courseImportAssignmentCmd = &cobra.Command{
	Use:   "import-assignment <course-id> <module-code> <file.json>",
	Short: "Publish a quiz assignment from a JSON envelope, verbatim, and verify it by read-back",
	Long: `Publish a quiz assignment to a course module without a module directory.

The file is a quiz envelope — {"type": "quiz", "version": 1, "passThreshold": N,
"questions": [...]} — exactly as the Andamio app stores and grades it. It is
validated before any request with the union of the rules the FCB Fan Campus app and
app.andamio.io enforce: type and
version, a non-empty questions array, unique question ids, non-blank prompts, at least two options
per question with unique non-empty values and non-blank labels, a correctValue matching one option, a
passThreshold in 1..len(questions), and an intro that is a Tiptap doc if present.
Every violated rule is listed. There is no bypass flag. A Tiptap document is
refused: author it as assignment.md in a module directory and use 'course import'.

Only the assignment is sent. Lessons, SLTs and the introduction are untouched.
The existing assignment's title, description, image and video URLs are kept
unless --title / --description override them; a module with no assignment yet
requires --title. Assignments are editable in any module status — only SLTs
lock after DRAFT — so this works on DRAFT, APPROVED, PENDING_TX and ON_CHAIN
modules alike.

After the update the module is re-fetched and the stored content_json is
deep-compared to the file. A mismatch, a degraded (206) read-back, or a
read-back request that fails for any reason exits 1 with kind "verify" under
--output json: the update WAS applied and should be inspected.

Examples:
  andamio course import-assignment <course-id> 101 quiz.json --dry-run
  andamio course import-assignment <course-id> 101 quiz.json --title "Module Quiz"
  andamio course import-assignment 101 quiz.json --course "Fan Campus"
  andamio course import-assignment <course-id> 101 quiz.json --output json

Requires user authentication via 'andamio user login'.`,
	Args: cobra.RangeArgs(2, 3),
	PreRunE: func(cmd *cobra.Command, args []string) error {
		return requireUserAuth()
	},
	RunE: runCourseImportAssignment,
}

// ImportAssignmentEnvelope is the stable --output json contract for
// `course import-assignment`. Verified is true only after the read-back
// deep-compare passed; a dry run reports DryRun true and Verified false.
type ImportAssignmentEnvelope struct {
	CourseID     string                  `json:"course_id"`
	ModuleCode   string                  `json:"module_code"`
	ModuleStatus string                  `json:"module_status"`
	Assignment   ImportAssignmentSummary `json:"assignment"`
	DryRun       bool                    `json:"dry_run,omitempty"`
	Verified     bool                    `json:"verified"`
}

// ImportAssignmentSummary digests what was (or would be) published. TitleSource
// is "flag" when --title supplied the title and "existing" when it came from
// the module's current assignment.
type ImportAssignmentSummary struct {
	Title         string   `json:"title"`
	TitleSource   string   `json:"title_source"`
	QuestionCount int      `json:"question_count"`
	PassThreshold int      `json:"pass_threshold"`
	QuestionIDs   []string `json:"question_ids"`
}

// assignmentUpdatePayload is the whole request body: the module identity and
// the assignment, nothing else. Omitted top-level fields are left unchanged by
// the gateway, which is what keeps lessons, SLTs and the introduction out of
// reach of this command.
type assignmentUpdatePayload struct {
	CourseID         string          `json:"course_id"`
	CourseModuleCode string          `json:"course_module_code"`
	Assignment       assignmentInput `json:"assignment"`
}

// assignmentInput mirrors db-api's AggregateAssignmentInput. Every metadata
// field is carried because processAssignmentUpdate overwrites all of them
// from the input — an omitted description is nulled, not preserved (KTD5 in
// the #165 plan). ContentJSON is the file bytes: no re-encoding on the way out.
type assignmentInput struct {
	Title       string          `json:"title"`
	Description *string         `json:"description,omitempty"`
	ImageURL    *string         `json:"image_url,omitempty"`
	VideoURL    *string         `json:"video_url,omitempty"`
	ContentJSON json.RawMessage `json:"content_json"`
}

type importAssignmentOptions struct {
	CourseID    string
	ModuleCode  string
	FilePath    string
	Title       string
	Description string
	DryRun      bool
	ShowPayload bool
	Quiet       bool // suppress stderr progress (JSON mode)
}

func runCourseImportAssignment(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	isJSON := output.GetFormat() == output.FormatJSON

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	c := client.New(cfg)

	opts := importAssignmentOptions{Quiet: isJSON}
	opts.Title, _ = cmd.Flags().GetString("title")
	opts.Description, _ = cmd.Flags().GetString("description")
	opts.DryRun, _ = cmd.Flags().GetBool("dry-run")
	opts.ShowPayload, _ = cmd.Flags().GetBool("show-payload")

	if len(args) == 3 {
		opts.CourseID, opts.ModuleCode, opts.FilePath = args[0], args[1], args[2]
	} else {
		// import-assignment <module-code> <file.json> --course "Name".
		// resolveCourseID owns the "no course given" error, as it does for
		// course export's two-arg form.
		opts.ModuleCode, opts.FilePath = args[0], args[1]
		opts.CourseID, err = resolveCourseID(ctx, c, "", cmd)
		if err != nil {
			return err
		}
	}

	env, err := runImportAssignment(ctx, c, opts)
	if err != nil {
		return err
	}

	if isJSON {
		return output.PrintJSON(env)
	}
	a := env.Assignment
	ids := strings.Join(a.QuestionIDs, ", ")
	if env.DryRun {
		fmt.Printf("Dry-run: would publish quiz assignment %q (title from %s) to module %s: %d questions, threshold %d, ids %s. Nothing sent.\n",
			a.Title, a.TitleSource, env.ModuleCode, a.QuestionCount, a.PassThreshold, ids)
		return nil
	}
	fmt.Printf("Published quiz assignment %q to module %s (%s): %d questions, threshold %d, ids %s — verified by read-back.\n",
		a.Title, env.ModuleCode, env.ModuleStatus, a.QuestionCount, a.PassThreshold, ids)
	return nil
}

// runImportAssignment is the command body, split from cobra so tests can drive
// it against an httptest gateway. Order matters: parse and validate the file
// first (no request for a bad file), fetch the module, resolve title and
// metadata, then either stop (dry run) or POST and read back.
func runImportAssignment(ctx context.Context, c *client.Client, opts importAssignmentOptions) (*ImportAssignmentEnvelope, error) {
	raw, err := os.ReadFile(opts.FilePath)
	if err != nil {
		return nil, fmt.Errorf("quiz file: %w", err)
	}
	env, summary, err := parseQuizFile(raw, opts.FilePath)
	if err != nil {
		return nil, err
	}

	if !opts.Quiet {
		fmt.Fprintf(os.Stderr, "Fetching module %s from course %s...\n", opts.ModuleCode, opts.CourseID)
	}
	existing, err := fetchExistingModule(ctx, c, opts.CourseID, opts.ModuleCode)
	if err != nil {
		switch {
		case errors.Is(err, errDegradedRead):
			// db-api may be the missing backend: the module may exist with an
			// assignment the list could not show. Inferring "no existing
			// assignment" would null its metadata on the write (R7a).
			return nil, fmt.Errorf("module state could not be read reliably: %w; not sending. Retry when the gateway is healthy", err)
		case errors.Is(err, errModuleNotFound):
			return nil, &apierr.NotFoundError{Message: err.Error() + ". Run 'andamio course modules " + opts.CourseID + " --output json' to list the course's modules"}
		}
		return nil, err
	}

	title, titleSource := opts.Title, "flag"
	if title == "" {
		titleSource = "existing"
		if existing.Assignment != nil {
			title, _ = existing.Assignment["title"].(string)
		}
		if title == "" {
			return nil, fmt.Errorf("title required for a module with no existing assignment: pass --title")
		}
	}

	input := assignmentInput{
		Title:       title,
		Description: firstNonEmpty(opts.Description, existingString(existing.Assignment, "description")),
		ImageURL:    firstNonEmpty(existingString(existing.Assignment, "image_url")),
		VideoURL:    firstNonEmpty(existingString(existing.Assignment, "video_url")),
		ContentJSON: json.RawMessage(raw),
	}
	payload := assignmentUpdatePayload{
		CourseID:         opts.CourseID,
		CourseModuleCode: opts.ModuleCode,
		Assignment:       input,
	}

	result := &ImportAssignmentEnvelope{
		CourseID:     opts.CourseID,
		ModuleCode:   opts.ModuleCode,
		ModuleStatus: existing.Status,
		Assignment: ImportAssignmentSummary{
			Title:         title,
			TitleSource:   titleSource,
			QuestionCount: summary.QuestionCount,
			PassThreshold: summary.PassThreshold,
			QuestionIDs:   summary.QuestionIDs,
		},
	}

	if opts.DryRun {
		result.DryRun = true
		if opts.ShowPayload && !opts.Quiet {
			pretty, err := json.MarshalIndent(payload, "", "  ")
			if err != nil {
				return nil, fmt.Errorf("failed to marshal payload: %w", err)
			}
			fmt.Fprintln(os.Stderr, "Dry-run payload (not sent):")
			fmt.Fprintln(os.Stderr, string(pretty))
		}
		return result, nil
	}

	if !opts.Quiet {
		fmt.Fprintf(os.Stderr, "Publishing quiz assignment (%d questions, threshold %d)...\n", summary.QuestionCount, summary.PassThreshold)
	}
	var resp map[string]interface{}
	if err := c.Post(ctx, "/api/v2/course/teacher/course-module/update", payload, &resp); err != nil {
		return nil, fmt.Errorf("failed to update module: %w", err)
	}

	// Read-back. From here on the module has been modified, so every failure
	// message says so — a caller must never read one as "nothing happened".
	if !opts.Quiet {
		fmt.Fprintln(os.Stderr, "Reading back to verify...")
	}
	stored, err := fetchExistingModule(ctx, c, opts.CourseID, opts.ModuleCode)
	if err != nil {
		// Every failure after the accepted POST is kind verify: "applied but
		// unconfirmed" is the outcome a caller must branch on, whatever the
		// cause. A connection reset here is not "the request never reached
		// the service" and an expired token is not "unauthenticated" — the
		// module changed. The cause stays inspectable via Unwrap.
		msg := fmt.Sprintf("the read-back request failed (%v); the stored value could not be confirmed", err)
		switch {
		case errors.Is(err, errDegradedRead):
			msg = fmt.Sprintf("the read-back was degraded (%v); the stored value could not be confirmed", err)
		case errors.Is(err, errModuleNotFound):
			msg = fmt.Sprintf("the read-back did not return the module (%v); the stored value could not be confirmed", err)
		}
		return nil, &apierr.VerifyError{Path: "assignment.content_json", Message: msg, Err: err}
	}
	if err := verifyStoredAssignment(env, input, stored.Assignment); err != nil {
		return nil, err
	}

	result.Verified = true
	return result, nil
}

// verifyStoredAssignment deep-compares what was sent with what the gateway now
// returns. content_json is compared structurally (the gateway re-serializes
// from jsonb, so bytes are not a property it offers); the metadata fields are
// compared as strings with absent and empty treated alike.
func verifyStoredAssignment(sent map[string]interface{}, input assignmentInput, stored map[string]interface{}) error {
	if stored == nil {
		return &apierr.VerifyError{Path: "assignment", Message: "the module has no assignment after the update"}
	}
	if !reflect.DeepEqual(stored["content_json"], sent) {
		return &apierr.VerifyError{Path: "assignment.content_json", Message: "the stored value did not read back identical to the file"}
	}
	fields := []struct {
		name string
		sent *string
	}{
		{"title", &input.Title},
		{"description", input.Description},
		{"image_url", input.ImageURL},
		{"video_url", input.VideoURL},
	}
	for _, f := range fields {
		want := ""
		if f.sent != nil {
			want = *f.sent
		}
		got, _ := stored[f.name].(string)
		if got != want {
			return &apierr.VerifyError{Path: "assignment." + f.name, Message: fmt.Sprintf("sent %q, stored %q", want, got)}
		}
	}
	return nil
}

// existingString reads a non-empty string field from the existing assignment
// (nil-safe), or "" when absent.
func existingString(assignment map[string]interface{}, field string) string {
	if assignment == nil {
		return ""
	}
	v, _ := assignment[field].(string)
	return v
}

// firstNonEmpty returns a pointer to the first non-empty candidate, or nil when
// all are empty so the JSON field is omitted.
func firstNonEmpty(candidates ...string) *string {
	for _, s := range candidates {
		if s != "" {
			return &s
		}
	}
	return nil
}
