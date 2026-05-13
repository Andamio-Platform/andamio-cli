package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/Andamio-Platform/andamio-cli/internal/config"
	"github.com/Andamio-Platform/andamio-cli/internal/output"
)

// =============================================================================
// renderTeacherAssignmentsListText — pure rendering coverage
// =============================================================================
//
// These tests pin the column layout introduced by issue #93: a Status column
// between SOURCE and COURSE ID, populated from content.commitment_status. The
// renderer is split from the Cobra handler so the column logic can be tested
// without httptest. Pattern mirrors renderQualifiedContributorsText in
// project_manager_ops_test.go.

// renderRow assembles a single result item with the given fields. content is
// optional — passing nil simulates the no-course-filter / on-chain-only row
// shape where content.commitment_status is absent entirely.
func renderRow(student, module, source, courseID string, content map[string]interface{}) map[string]interface{} {
	row := map[string]interface{}{
		"student_alias":      student,
		"course_module_code": module,
		"source":             source,
		"course_id":          courseID,
	}
	if content != nil {
		row["content"] = content
	}
	return row
}

func TestRenderTeacherAssignmentsListText_HappyPath_StatusColumnPresent(t *testing.T) {
	var stdout bytes.Buffer
	data := []interface{}{
		renderRow("ada", "101", "merged", "C1", map[string]interface{}{
			"commitment_status": "AWAITING_SUBMISSION",
		}),
	}
	if err := renderTeacherAssignmentsListText(data, &stdout); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "STATUS") {
		t.Errorf("output missing STATUS column header:\n%s", out)
	}
	if !strings.Contains(out, "AWAITING_SUBMISSION") {
		t.Errorf("output missing AWAITING_SUBMISSION value:\n%s", out)
	}
	// COURSE ID must still appear after STATUS — verifies column ordering.
	statusIdx := strings.Index(out, "AWAITING_SUBMISSION")
	courseIdx := strings.LastIndex(out, "C1")
	if statusIdx < 0 || courseIdx < 0 || courseIdx <= statusIdx {
		t.Errorf("STATUS should appear before COURSE ID in row; got status at %d, course at %d:\n%s", statusIdx, courseIdx, out)
	}
}

func TestRenderTeacherAssignmentsListText_LongerEnumRendersVerbatim(t *testing.T) {
	// CREDENTIAL_CLAIMED is 18 runes — within the 20 minimum width, no expansion needed.
	// This locks the verbatim contract for the widest commonly seen enum.
	var stdout bytes.Buffer
	data := []interface{}{
		renderRow("alan", "201", "merged", "C2", map[string]interface{}{
			"commitment_status": "CREDENTIAL_CLAIMED",
		}),
	}
	if err := renderTeacherAssignmentsListText(data, &stdout); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !strings.Contains(stdout.String(), "CREDENTIAL_CLAIMED") {
		t.Errorf("CREDENTIAL_CLAIMED not rendered verbatim:\n%s", stdout.String())
	}
}

func TestRenderTeacherAssignmentsListText_PendingTxStatusRendersVerbatim(t *testing.T) {
	// PENDING_TX_* is a family of transient statuses; the contract is "no
	// filtering, no aliasing" regardless of which variant the gateway emits.
	// Using a representative value documents the principle without depending
	// on an exact server-side string we don't control.
	var stdout bytes.Buffer
	data := []interface{}{
		renderRow("turing", "301", "merged", "C3", map[string]interface{}{
			"commitment_status": "PENDING_TX_SUBMIT",
		}),
	}
	if err := renderTeacherAssignmentsListText(data, &stdout); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !strings.Contains(stdout.String(), "PENDING_TX_SUBMIT") {
		t.Errorf("PENDING_TX_SUBMIT not rendered verbatim:\n%s", stdout.String())
	}
}

func TestRenderTeacherAssignmentsListText_MissingContentKey_RendersPlaceholder(t *testing.T) {
	// The no-course-filter response shape returns lightweight on-chain-only
	// rows where the nested content object is absent entirely. The Status
	// column must render the placeholder rather than blanking the cell.
	var stdout bytes.Buffer
	data := []interface{}{
		renderRow("ada", "101", "chain_only", "C1", nil),
	}
	if err := renderTeacherAssignmentsListText(data, &stdout); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !strings.Contains(stdout.String(), statusEmpty) {
		t.Errorf("expected placeholder %q for missing content; output:\n%s", statusEmpty, stdout.String())
	}
}

func TestRenderTeacherAssignmentsListText_EmptyCommitmentStatus_RendersPlaceholder(t *testing.T) {
	// content is present but commitment_status is missing or empty. Same
	// behavior as a missing content key: placeholder, no fallback enum.
	var stdout bytes.Buffer
	data := []interface{}{
		renderRow("ada", "101", "db_only", "C1", map[string]interface{}{
			// commitment_status omitted
			"evidence": map[string]interface{}{"foo": "bar"},
		}),
	}
	if err := renderTeacherAssignmentsListText(data, &stdout); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !strings.Contains(stdout.String(), statusEmpty) {
		t.Errorf("expected placeholder %q for empty commitment_status; output:\n%s", statusEmpty, stdout.String())
	}
}

func TestRenderTeacherAssignmentsListText_NoFallbackToOnChainStatus(t *testing.T) {
	// Even when on_chain_status is populated on the row, the Status column
	// must NOT fall back to it. Mixing two enums in one column would alias
	// across them and violate the issue's "no aliasing" contract.
	var stdout bytes.Buffer
	row := renderRow("ada", "101", "chain_only", "C1", nil)
	row["on_chain_status"] = "SOME_ONCHAIN_VALUE"
	data := []interface{}{row}
	if err := renderTeacherAssignmentsListText(data, &stdout); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if strings.Contains(stdout.String(), "SOME_ONCHAIN_VALUE") {
		t.Errorf("Status column must not leak on_chain_status; output:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), statusEmpty) {
		t.Errorf("expected placeholder %q; output:\n%s", statusEmpty, stdout.String())
	}
}

func TestRenderTeacherAssignmentsListText_MixedStatuses_PreserveAlignment(t *testing.T) {
	// Multiple rows with a mix of populated, empty, and longer status values
	// in one table. Each value must render verbatim; the COURSE ID column
	// must appear at a consistent offset so each row remains parsable.
	var stdout bytes.Buffer
	data := []interface{}{
		renderRow("ada", "101", "merged", "courseA", map[string]interface{}{
			"commitment_status": "SUBMITTED",
		}),
		renderRow("alan", "102", "chain_only", "courseB", nil),
		renderRow("turing", "103", "merged", "courseC", map[string]interface{}{
			"commitment_status": "REFUSED",
		}),
	}
	if err := renderTeacherAssignmentsListText(data, &stdout); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	out := stdout.String()
	for _, want := range []string{"SUBMITTED", "REFUSED", statusEmpty, "courseA", "courseB", "courseC"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in mixed output:\n%s", want, out)
		}
	}

	// Locate the column position of each COURSE ID in its row. With
	// rune-aware padding they must all land in the same column even though
	// the placeholder is a multi-byte em-dash.
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) < 5 {
		t.Fatalf("expected header + separator + 3 data rows, got %d lines:\n%s", len(lines), out)
	}
	dataLines := lines[2:]
	type ciCol struct {
		line  string
		col   int
		runes int
	}
	cols := make([]ciCol, len(dataLines))
	for i, line := range dataLines {
		idx := strings.LastIndex(line, "course")
		if idx < 0 {
			t.Fatalf("could not find course ID in line: %q", line)
		}
		// Rune column = number of runes before the byte offset idx.
		runes := []rune(line[:idx])
		cols[i] = ciCol{line: line, col: idx, runes: len(runes)}
	}
	for i := 1; i < len(cols); i++ {
		if cols[i].runes != cols[0].runes {
			t.Errorf("COURSE ID column misaligned: line 0 starts at rune col %d, line %d at %d. Em-dash row likely under-padded.\n  line 0: %q\n  line %d: %q",
				cols[0].runes, i, cols[i].runes, cols[0].line, i, cols[i].line)
		}
	}
}

func TestRenderTeacherAssignmentsListText_ColumnExpandsForLongerStatus(t *testing.T) {
	// If a future enum value exceeds the 20-rune floor, the column must
	// expand rather than truncate. We pin the expand-not-slice behavior so
	// "verbatim" is preserved even when the gateway grows the vocabulary.
	const longStatus = "A_HYPOTHETICAL_FUTURE_STATUS_LONGER_THAN_TWENTY"
	var stdout bytes.Buffer
	data := []interface{}{
		renderRow("ada", "101", "merged", "C1", map[string]interface{}{
			"commitment_status": longStatus,
		}),
	}
	if err := renderTeacherAssignmentsListText(data, &stdout); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !strings.Contains(stdout.String(), longStatus) {
		t.Errorf("long status was sliced or relabeled; output:\n%s", stdout.String())
	}
}

// =============================================================================
// fetchTeacherAssignmentsList — JSON pass-through coverage (R2)
// =============================================================================
//
// These tests exercise the wire path with an httptest stub. They guard the
// issue's R2 acceptance bullet: JSON output mode must include the status
// field unchanged. Because fetchTeacherAssignmentsList decodes the body into
// map[string]interface{} (not a typed struct), any future addition to the
// gateway envelope flows through automatically — these tests prove the
// nested status fields specifically survive the round trip today.

func stubTeacherAssignmentsServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *client.Client) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c := client.New(&config.Config{BaseURL: srv.URL})
	return srv, c
}

func TestFetchTeacherAssignmentsList_RequestShape_WithCourseSendsCourseIDInBody(t *testing.T) {
	var gotPath, gotMethod, gotCT string
	var gotBody []byte
	_, c := stubTeacherAssignmentsServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		gotCT = r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"data": []interface{}{}})
	})

	_, err := fetchTeacherAssignmentsList(context.Background(), c, "the-course-id")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if gotMethod != "POST" {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if want := "/api/v2/course/teacher/assignment-commitments/list"; gotPath != want {
		t.Errorf("path = %q, want %q", gotPath, want)
	}
	if !strings.Contains(gotCT, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", gotCT)
	}
	var parsed map[string]string
	if err := json.Unmarshal(gotBody, &parsed); err != nil {
		t.Fatalf("request body not valid JSON: %v; raw=%q", err, gotBody)
	}
	if parsed["course_id"] != "the-course-id" {
		t.Errorf("body.course_id = %q, want %q", parsed["course_id"], "the-course-id")
	}
}

func TestFetchTeacherAssignmentsList_RequestShape_WithoutCourseOmitsBody(t *testing.T) {
	var gotBody []byte
	_, c := stubTeacherAssignmentsServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"data": []interface{}{}})
	})

	if _, err := fetchTeacherAssignmentsList(context.Background(), c, ""); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(gotBody) != 0 {
		t.Errorf("body = %q, want empty (no --course filter -> bodyless POST)", string(gotBody))
	}
}

func TestFetchTeacherAssignmentsList_JSONPassThrough_PreservesNestedStatusFields(t *testing.T) {
	// This is the R2 guardrail. Construct a gateway response with both the
	// nested content.commitment_status and the top-level on_chain_status, then
	// assert both survive the decode/return cycle. If a future refactor
	// introduces a typed response struct, this test catches the moment those
	// fields are silently dropped by missing JSON tags.
	const responseBody = `{
        "data": [
            {
                "course_id": "C1",
                "course_module_code": "101",
                "student_alias": "ada",
                "source": "merged",
                "on_chain_status": "ENROLLED",
                "content": {
                    "commitment_status": "AWAITING_SUBMISSION",
                    "assignment_evidence_hash": "abc123"
                }
            }
        ]
    }`
	_, c := stubTeacherAssignmentsServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, responseBody)
	})

	resp, err := fetchTeacherAssignmentsList(context.Background(), c, "C1")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	data, ok := resp["data"].([]interface{})
	if !ok || len(data) != 1 {
		t.Fatalf("data shape unexpected: %v", resp)
	}
	row, ok := data[0].(map[string]interface{})
	if !ok {
		t.Fatalf("row not a map: %v", data[0])
	}
	if got := row["on_chain_status"]; got != "ENROLLED" {
		t.Errorf("on_chain_status = %v, want ENROLLED", got)
	}
	content, ok := row["content"].(map[string]interface{})
	if !ok {
		t.Fatalf("content not a map: %v", row["content"])
	}
	if got := content["commitment_status"]; got != "AWAITING_SUBMISSION" {
		t.Errorf("content.commitment_status = %v, want AWAITING_SUBMISSION", got)
	}
	if got := content["assignment_evidence_hash"]; got != "abc123" {
		t.Errorf("content.assignment_evidence_hash = %v, want abc123 (sibling fields must also survive)", got)
	}
}

func TestFetchTeacherAssignmentsList_EndToEndText_RendersStatusFromWire(t *testing.T) {
	// Compose fetch + render to prove the full pipeline emits a STATUS column
	// populated from the wire response. This is the integration scenario the
	// renderer unit tests alone can't prove.
	const responseBody = `{
        "data": [
            {
                "course_id": "C1",
                "course_module_code": "101",
                "student_alias": "ada",
                "source": "merged",
                "content": {"commitment_status": "ACCEPTED"}
            }
        ]
    }`
	_, c := stubTeacherAssignmentsServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, responseBody)
	})

	resp, err := fetchTeacherAssignmentsList(context.Background(), c, "C1")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	data, _ := resp["data"].([]interface{})
	var stdout bytes.Buffer
	if err := renderTeacherAssignmentsListText(data, &stdout); err != nil {
		t.Fatalf("render: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "STATUS") {
		t.Errorf("end-to-end text missing STATUS column:\n%s", out)
	}
	if !strings.Contains(out, "ACCEPTED") {
		t.Errorf("end-to-end text missing wire status ACCEPTED:\n%s", out)
	}
}

// =============================================================================
// runTeacherAssignmentsList — full Cobra handler integration (plan R2)
// =============================================================================
//
// These tests drive runTeacherAssignmentsList through its Cobra entry point —
// the same path real users hit. They exercise the format-dispatch branch and
// the empty-data guard that the renderer-unit and fetch-unit tests bypass.
// Pattern mirrors dev_test.go (HOME tempdir + config.Save + output.SetFormat
// + captureStdout).

// teacherAssignmentsHandlerEnv stubs the gateway, sandboxes config to a
// tempdir HOME, and saves a Config with BaseURL pointed at the stub. Returns
// nothing the caller needs to keep — all wiring lives in the saved config
// that the handler reads via config.Load().
func teacherAssignmentsHandlerEnv(t *testing.T, body string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)

	t.Setenv("HOME", t.TempDir())
	if err := config.Save(&config.Config{BaseURL: srv.URL}); err != nil {
		t.Fatalf("seed config: %v", err)
	}
}

func TestRunTeacherAssignmentsList_JSONOutput_HandlerPassesGatewayShapeVerbatim(t *testing.T) {
	// R2 regression guard at the handler level: full path through the
	// format-dispatch branch in runTeacherAssignmentsList, not just the
	// extracted fetch function. If a future refactor reorders the format
	// check or wraps the gateway envelope before printing, this test fails.
	const responseBody = `{
        "data": [
            {
                "course_id": "C1",
                "course_module_code": "101",
                "student_alias": "ada",
                "source": "merged",
                "on_chain_status": "ENROLLED",
                "content": {
                    "commitment_status": "AWAITING_SUBMISSION",
                    "assignment_evidence_hash": "abc123"
                }
            }
        ]
    }`
	teacherAssignmentsHandlerEnv(t, responseBody)

	cmd := teacherAssignmentsListCmd
	cmd.SetContext(context.Background())
	if err := cmd.Flags().Set("course", "C1"); err != nil {
		t.Fatalf("set --course flag: %v", err)
	}
	t.Cleanup(func() { _ = cmd.Flags().Set("course", "") })

	captured := captureStdout(t, func() {
		_ = output.SetFormat("json")
		t.Cleanup(func() { _ = output.SetFormat("text") })
		if err := cmd.RunE(cmd, []string{}); err != nil {
			t.Fatalf("handler: %v", err)
		}
	})

	var got map[string]interface{}
	if err := json.Unmarshal([]byte(captured), &got); err != nil {
		t.Fatalf("handler stdout is not JSON: %v\nbytes: %s", err, captured)
	}

	data, ok := got["data"].([]interface{})
	if !ok || len(data) != 1 {
		t.Fatalf("data shape unexpected: %v", got)
	}
	row, _ := data[0].(map[string]interface{})
	if row["on_chain_status"] != "ENROLLED" {
		t.Errorf("on_chain_status = %v, want ENROLLED (top-level status field must survive handler dispatch)", row["on_chain_status"])
	}
	content, ok := row["content"].(map[string]interface{})
	if !ok {
		t.Fatalf("content not a nested map: %v", row["content"])
	}
	if content["commitment_status"] != "AWAITING_SUBMISSION" {
		t.Errorf("content.commitment_status = %v, want AWAITING_SUBMISSION (R2: nested status must survive)", content["commitment_status"])
	}
	if content["assignment_evidence_hash"] != "abc123" {
		t.Errorf("content.assignment_evidence_hash = %v, want abc123 (R2: sibling nested fields must also survive)", content["assignment_evidence_hash"])
	}
}

func TestRunTeacherAssignmentsList_TextOutput_HandlerRendersStatusColumn(t *testing.T) {
	// End-to-end text path: full handler exercises format dispatch and the
	// renderer. Catches regressions where a future change short-circuits the
	// text branch (e.g., flips the format check to == FormatJSON, which would
	// drop CSV/markdown through to text silently).
	const responseBody = `{
        "data": [
            {
                "course_id": "C1",
                "course_module_code": "101",
                "student_alias": "ada",
                "source": "merged",
                "content": {"commitment_status": "ACCEPTED"}
            }
        ]
    }`
	teacherAssignmentsHandlerEnv(t, responseBody)

	cmd := teacherAssignmentsListCmd
	cmd.SetContext(context.Background())
	if err := cmd.Flags().Set("course", "C1"); err != nil {
		t.Fatalf("set --course flag: %v", err)
	}
	t.Cleanup(func() { _ = cmd.Flags().Set("course", "") })

	captured := captureStdout(t, func() {
		_ = output.SetFormat("text")
		if err := cmd.RunE(cmd, []string{}); err != nil {
			t.Fatalf("handler: %v", err)
		}
	})

	if !strings.Contains(captured, "STATUS") {
		t.Errorf("handler text output missing STATUS column:\n%s", captured)
	}
	if !strings.Contains(captured, "ACCEPTED") {
		t.Errorf("handler text output missing wire status ACCEPTED:\n%s", captured)
	}
}

// =============================================================================
// padRunes — small invariant check on the rune-aware padder
// =============================================================================
//
// padRunes lives in helpers.go alongside truncateUTF8 (both are rune-aware
// string utilities for fixed-width table rendering). These tests cover its
// invariants and stay in this file because the original need for the helper
// was the Status column's em-dash placeholder.

func TestPadRunes_PadsMultiByteToVisualWidth(t *testing.T) {
	// "—" is 3 bytes but 1 rune. padRunes must pad to the requested rune
	// width so adjacent columns stay aligned. Without this helper, fmt's
	// %-Ns byte-counting would underpad em-dash rows.
	got := padRunes("—", 5)
	wantRunes := 5
	if r := len([]rune(got)); r != wantRunes {
		t.Errorf("padRunes(\"—\", 5) produced %d runes, want %d; got=%q", r, wantRunes, got)
	}
	if !strings.HasPrefix(got, "—") {
		t.Errorf("padRunes should prefix with the input; got=%q", got)
	}
}

func TestPadRunes_NoOpWhenAlreadyAtOrAboveWidth(t *testing.T) {
	if got := padRunes("EXACTLY_TEN", 10); got != "EXACTLY_TEN" {
		t.Errorf("padRunes did not pass through equal-width string: %q", got)
	}
	if got := padRunes("LONGER_THAN_FIVE", 5); got != "LONGER_THAN_FIVE" {
		t.Errorf("padRunes truncated a longer-than-width string: %q", got)
	}
}
