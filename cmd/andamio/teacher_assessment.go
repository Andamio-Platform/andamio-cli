package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/Andamio-Platform/andamio-cli/internal/config"
	"github.com/Andamio-Platform/andamio-cli/internal/output"
	"github.com/spf13/cobra"
)

// assessEndpoint is the gateway path that builds an assessment transaction.
// The client prefixes /api, matching tx build.
const assessEndpoint = "/api/v2/tx/course/teacher/assignments/assess"

// maxDecisionsFileSize caps --decisions-file at 1 MB, mirroring the evidence
// file cap that preceded it. A decision list is a few bytes per student.
const maxDecisionsFileSize = 1 << 20

// AssessmentDecision is one teacher decision about one learner's submission.
//
// Alias is the LEARNER's on-chain alias. The teacher's own alias travels
// separately, at the top level of the request — see buildAssessmentPayload.
type AssessmentDecision struct {
	Alias   string `json:"alias"`
	Outcome string `json:"outcome"`
}

// AssessmentBuildEnvelope is the stable --output json contract for
// `teacher assessment build`. Errors render via the global {"error": ...}
// path in main.go.
//
// The point of this envelope is that UnsignedTx and Decisions travel together.
// An assessment decision should not be automated end to end: a program can
// recommend one, but a person approves it. That approval is only meaningful if
// the person can see what they are approving — and before 1.0 they could not.
// The agent built the transaction with `tx build`, got back opaque CBOR, and
// then described in its own prose what that CBOR supposedly encoded. The human
// was trusting the narration, not the artifact.
//
// Decisions closes that gap by carrying the decision set in the same output,
// from the same command, as the transaction it produced.
//
// Known limitation, stated plainly: Decisions is an echo of the request the
// CLI sent, not a decode of the transaction the gateway returned. It proves
// what was asked for, not what was built. Decoding the CBOR would require
// interpreting Plutus datums and is deliberately out of scope for 1.0 — until
// then, this envelope narrows the trust gap rather than closing it.
type AssessmentBuildEnvelope struct {
	UnsignedTx    string               `json:"unsigned_tx"`
	CourseID      string               `json:"course_id"`
	TeacherAlias  string               `json:"teacher_alias"`
	Decisions     []AssessmentDecision `json:"decisions"`
	DecisionCount int                  `json:"decision_count"`
}

var teacherAssessmentCmd = &cobra.Command{
	Use:   "assessment",
	Short: "Build assessment transactions for review (teacher role)",
}

var teacherAssessmentBuildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build an assessment transaction without signing or submitting it",
	Long: `Build a course assignment assessment transaction and stop.

Nothing is signed and nothing is submitted. The transaction is returned
unsigned so a person can review the decisions it carries before approving
them — then sign and submit as separate steps:

  andamio tx sign --tx <unsigned-tx> --skey <path>
  andamio tx submit --tx <signed-tx>

'andamio tx run' remains available for anyone who wants build, sign, submit
and confirm in a single step.

One transaction carries every decision. Include accepts AND refuses: the
transaction records the teacher's decision for every submission assessed,
not only the passing ones.

Decisions are given as <student-alias>=<accept|refuse>, repeatable, or as a
JSON file of [{"alias": "...", "outcome": "accept"}, ...].

WHAT --output json RETURNS

  .unsigned_tx     CBOR hex, ready for 'andamio tx sign'
  .course_id       course the assessment applies to
  .teacher_alias   alias the assessment is signed on behalf of
  .decisions[]     the decision set, in the order given
  .decision_count  length of .decisions

The decisions echoed back are the request this command sent — not a decode
of the returned transaction. They let a reviewer check the decision set and
the transaction in one place, from one command, instead of trusting a
separate summary. They do not prove what the gateway built.

Examples:
  andamio teacher assessment build --course-id <id> --alias teacher-01 \
    --decision student-01=accept --decision student-02=refuse

  andamio teacher assessment build --course-id <id> --alias teacher-01 \
    --decisions-file decisions.json --output json`,
	// Auth is enforced by teacherCmd's PersistentPreRunE, which cobra resolves
	// by walking up from the leaf. Redeclaring it here would shadow the parent
	// with an identical check.
	RunE: runTeacherAssessmentBuild,
}

func init() {
	teacherCmd.AddCommand(teacherAssessmentCmd)
	teacherAssessmentCmd.AddCommand(teacherAssessmentBuildCmd)

	teacherAssessmentBuildCmd.Flags().String("course-id", "", "Course ID (required)")
	teacherAssessmentBuildCmd.MarkFlagRequired("course-id")
	teacherAssessmentBuildCmd.Flags().String("alias", "", "Your on-chain teacher alias (required)")
	teacherAssessmentBuildCmd.MarkFlagRequired("alias")
	teacherAssessmentBuildCmd.Flags().StringArray("decision", nil,
		"Decision as <student-alias>=<accept|refuse> (repeatable)")
	teacherAssessmentBuildCmd.Flags().String("decisions-file", "",
		"Path to a JSON file of [{\"alias\":\"...\",\"outcome\":\"accept\"}]")
}

// buildAssessmentPayload assembles the request body for the assess endpoint.
//
// Field naming is the thing to get right here, and it is genuinely confusing:
// the top-level `alias` is the TEACHER, while each `assignment_decisions[].alias`
// is a LEARNER. Same key, two nesting levels, two different people. There is
// deliberately no module_code at any level — the protocol derives the module
// from the learner's on-chain commitment.
//
// Schema reference: .claude/skills/assess-assignment/SKILL.md.
func buildAssessmentPayload(courseID, teacherAlias string, decisions []AssessmentDecision) map[string]interface{} {
	wire := make([]map[string]string, 0, len(decisions))
	for _, d := range decisions {
		wire = append(wire, map[string]string{
			"alias":   d.Alias,
			"outcome": d.Outcome,
		})
	}
	return map[string]interface{}{
		"alias":                teacherAlias,
		"course_id":            courseID,
		"assignment_decisions": wire,
	}
}

// parseDecisionFlags turns repeated --decision alias=outcome flags into
// decisions, preserving flag order.
func parseDecisionFlags(flags []string) ([]AssessmentDecision, error) {
	decisions := make([]AssessmentDecision, 0, len(flags))
	for _, flag := range flags {
		alias, outcome, found := strings.Cut(flag, "=")
		if !found {
			return nil, fmt.Errorf("invalid --decision %q: expected alias=outcome, e.g. student-01=accept", flag)
		}
		decisions = append(decisions, AssessmentDecision{
			Alias:   strings.TrimSpace(alias),
			Outcome: strings.TrimSpace(outcome),
		})
	}
	return validateDecisions(decisions)
}

// readDecisionsFile loads a JSON decision list, for batches too large to pass
// as flags — the shape an agent naturally produces.
func readDecisionsFile(path string) ([]AssessmentDecision, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read decisions file %s: %w", path, err)
	}
	if info.Size() > maxDecisionsFileSize {
		return nil, fmt.Errorf("decisions file %s is too large (%d bytes, max %d)", path, info.Size(), maxDecisionsFileSize)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read decisions file %s: %w", path, err)
	}

	var decisions []AssessmentDecision
	if err := json.Unmarshal(data, &decisions); err != nil {
		return nil, fmt.Errorf("failed to parse decisions file %s: %w\n\nExpected: [{\"alias\": \"student-01\", \"outcome\": \"accept\"}]", path, err)
	}
	for i := range decisions {
		decisions[i].Alias = strings.TrimSpace(decisions[i].Alias)
		decisions[i].Outcome = strings.TrimSpace(decisions[i].Outcome)
	}
	return validateDecisions(decisions)
}

// validateDecisions enforces the rules that protect a credential decision from
// being wrong in a way nobody notices.
//
// The duplicate check is the load-bearing one. Two entries for the same learner
// with opposing outcomes is not a preference to resolve by taking the last —
// it means the caller lost track of its own decision set, and silently picking
// one would put a credential decision on-chain that nobody made.
func validateDecisions(decisions []AssessmentDecision) ([]AssessmentDecision, error) {
	if len(decisions) == 0 {
		return nil, fmt.Errorf("at least one decision is required\n\nList commitments awaiting review with:\n  andamio teacher assignments list --course <course-id> --output json")
	}

	seen := make(map[string]bool, len(decisions))
	for _, d := range decisions {
		if d.Alias == "" {
			return nil, fmt.Errorf("decision is missing a student alias")
		}
		switch d.Outcome {
		case "accept", "refuse":
		case "":
			return nil, fmt.Errorf("decision for %q is missing an outcome: must be accept or refuse", d.Alias)
		default:
			// Case-normalize only after an exact match fails, so a shell variable
			// arriving as ACCEPT works while an unrecognized word is still rejected.
			lowered := strings.ToLower(d.Outcome)
			if lowered != "accept" && lowered != "refuse" {
				return nil, fmt.Errorf("invalid outcome %q for %q: must be accept or refuse", d.Outcome, d.Alias)
			}
		}
		if seen[d.Alias] {
			return nil, fmt.Errorf("student %q appears more than once; each student gets exactly one decision", d.Alias)
		}
		seen[d.Alias] = true
	}

	normalized := make([]AssessmentDecision, len(decisions))
	for i, d := range decisions {
		normalized[i] = AssessmentDecision{Alias: d.Alias, Outcome: strings.ToLower(d.Outcome)}
	}
	return normalized, nil
}

func runTeacherAssessmentBuild(cmd *cobra.Command, args []string) error {
	courseID, _ := cmd.Flags().GetString("course-id")
	teacherAlias, _ := cmd.Flags().GetString("alias")
	decisionFlags, _ := cmd.Flags().GetStringArray("decision")
	decisionsFile, _ := cmd.Flags().GetString("decisions-file")
	isJSON := output.GetFormat() == output.FormatJSON

	if len(decisionFlags) > 0 && decisionsFile != "" {
		return fmt.Errorf("--decision and --decisions-file are mutually exclusive")
	}

	var decisions []AssessmentDecision
	var err error
	if decisionsFile != "" {
		decisions, err = readDecisionsFile(decisionsFile)
	} else {
		decisions, err = parseDecisionFlags(decisionFlags)
	}
	if err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	if !isJSON {
		fmt.Fprintf(os.Stderr, "Building assessment transaction for %d decision(s)...\n", len(decisions))
	}

	c := client.New(cfg)
	payload := buildAssessmentPayload(courseID, teacherAlias, decisions)

	var resp map[string]interface{}
	if err := c.Post(cmd.Context(), assessEndpoint, payload, &resp); err != nil {
		return fmt.Errorf("failed to build assessment transaction: %w", err)
	}

	unsignedTx, _ := resp["unsigned_tx"].(string)
	if unsignedTx == "" {
		return fmt.Errorf("gateway returned no unsigned_tx; nothing to sign")
	}

	envelope := AssessmentBuildEnvelope{
		UnsignedTx:    unsignedTx,
		CourseID:      courseID,
		TeacherAlias:  teacherAlias,
		Decisions:     decisions,
		DecisionCount: len(decisions),
	}

	if isJSON {
		return output.PrintJSON(envelope)
	}
	renderAssessmentBuild(os.Stdout, envelope)
	fmt.Fprintf(os.Stderr, "\nNothing has been signed or submitted. To proceed:\n"+
		"  andamio tx sign --tx <unsigned_tx> --skey <path>\n"+
		"  andamio tx submit --tx <signed_tx>\n")
	return nil
}

// renderAssessmentBuild writes the human-facing review view: the decisions
// first, the transaction second. That order is deliberate — the decisions are
// what a person can actually check, and burying them under a wall of CBOR
// invites approving the transaction without reading them.
//
// Takes an io.Writer for the same reason the other renderers in this package
// do: so the layout is testable without capturing os.Stdout.
func renderAssessmentBuild(w io.Writer, e AssessmentBuildEnvelope) {
	accepted, refused := countOutcomes(e.Decisions)

	fmt.Fprintf(w, "\nAssessment for course %s (as %s)\n\n", e.CourseID, e.TeacherAlias)
	fmt.Fprintf(w, "%-24s %s\n", "STUDENT", "DECISION")
	fmt.Fprintf(w, "%-24s %s\n", "-------", "--------")
	for _, d := range e.Decisions {
		fmt.Fprintf(w, "%-24s %s\n", truncateUTF8(d.Alias, 24), strings.ToUpper(d.Outcome))
	}
	fmt.Fprintf(w, "\n%d decision(s): %d accept, %d refuse\n", e.DecisionCount, accepted, refused)

	fmt.Fprintf(w, "\nUnsigned TX:\n%s\n", e.UnsignedTx)
}

func countOutcomes(decisions []AssessmentDecision) (accepted, refused int) {
	for _, d := range decisions {
		if d.Outcome == "accept" {
			accepted++
		} else {
			refused++
		}
	}
	return accepted, refused
}
