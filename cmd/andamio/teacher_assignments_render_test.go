package main

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/Andamio-Platform/andamio-cli/internal/config"
	"github.com/Andamio-Platform/andamio-cli/internal/output"
)

// assignmentRow builds a gateway commitment row. evidence may be nil.
func assignmentRow(alias, module, status string, evidence interface{}) map[string]interface{} {
	content := map[string]interface{}{"commitment_status": status}
	if evidence != nil {
		content["evidence"] = evidence
	}
	return map[string]interface{}{
		"course_id": "C1", "course_module_code": module, "student_alias": alias, "content": content,
	}
}

// enrichedRows runs rows through the same enrichment the fetch path applies.
func enrichedRows(rows ...map[string]interface{}) []interface{} {
	data := make([]interface{}, 0, len(rows))
	for _, r := range rows {
		data = append(data, r)
	}
	enrichCommitmentRows(map[string]interface{}{"data": data})
	return data
}

func readCSV(t *testing.T, s string) [][]string {
	t.Helper()
	records, err := csv.NewReader(strings.NewReader(s)).ReadAll()
	if err != nil {
		t.Fatalf("output is not valid CSV: %v\n%s", err, s)
	}
	return records
}

var longHeader = []string{"student_alias", "course_module_code", "status", "prompt_id", "label", "question", "answer"}

func TestRenderTeacherAssignmentsCSV_LongMixesPromptsAndWritten(t *testing.T) {
	data := enrichedRows(
		assignmentRow("ana", "102", "SUBMITTED", promptsEvidence(fcbAnswers...)),
		assignmentRow("jordi", "101", "SUBMITTED", tiptapDoc("Written work.")),
		assignmentRow("pau", "102", "AWAITING_SUBMISSION", nil),
		map[string]interface{}{"course_id": "C1", "course_module_code": "103", "student_alias": "summary"},
	)
	var buf bytes.Buffer
	if err := renderTeacherAssignmentsCSV(data, false, &buf); err != nil {
		t.Fatal(err)
	}
	got := readCSV(t, buf.String())
	want := [][]string{
		longHeader,
		{"ana", "102", "SUBMITTED", "c102-cause", "The cause", fcbAnswers[0][2], fcbAnswers[0][3]},
		{"ana", "102", "SUBMITTED", "c102-value", "The value", fcbAnswers[1][2], fcbAnswers[1][3]},
		{"ana", "102", "SUBMITTED", "c102-ask", "The ask", fcbAnswers[2][2], fcbAnswers[2][3]},
		{"jordi", "101", "SUBMITTED", "", "", "", "Written work."},
		{"pau", "102", "AWAITING_SUBMISSION", "", "", "", ""},
		{"summary", "103", "", "", "", "", ""},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("csv =\n%v\nwant\n%v", got, want)
	}
	if strings.Contains(buf.String(), "map[") {
		t.Errorf("csv contains a Go map dump:\n%s", buf.String())
	}
}

func TestRenderTeacherAssignmentsCSV_QuotesAnswers(t *testing.T) {
	answer := "First, \"quoted\" line.\nSecond line."
	data := enrichedRows(assignmentRow("ana", "102", "SUBMITTED",
		promptsEvidence([4]string{"c102-cause", "The cause", "Why?", answer})))
	var buf bytes.Buffer
	if err := renderTeacherAssignmentsCSV(data, false, &buf); err != nil {
		t.Fatal(err)
	}
	if got := readCSV(t, buf.String())[1][6]; got != answer {
		t.Errorf("answer = %q, want %q", got, answer)
	}
}

func TestRenderTeacherAssignmentsCSV_EmptyIsHeaderOnly(t *testing.T) {
	for _, wide := range []bool{false, true} {
		var buf bytes.Buffer
		if err := renderTeacherAssignmentsCSV(nil, wide, &buf); err != nil {
			t.Fatalf("wide=%v: %v", wide, err)
		}
		if got := readCSV(t, buf.String()); len(got) != 1 {
			t.Errorf("wide=%v: rows = %v, want header only", wide, got)
		}
	}
}

func TestRenderTeacherAssignmentsCSV_WideOneModule(t *testing.T) {
	data := enrichedRows(
		assignmentRow("ana", "102", "SUBMITTED", promptsEvidence(fcbAnswers...)),
		assignmentRow("marc", "102", "SUBMITTED", promptsEvidence(fcbAnswers[0], fcbAnswers[2])),
		assignmentRow("pau", "102", "AWAITING_SUBMISSION", nil),
	)
	var buf bytes.Buffer
	if err := renderTeacherAssignmentsCSV(data, true, &buf); err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"student_alias", "course_module_code", "status", "c102-cause", "c102-value", "c102-ask"},
		{"ana", "102", "SUBMITTED", fcbAnswers[0][3], fcbAnswers[1][3], fcbAnswers[2][3]},
		{"marc", "102", "SUBMITTED", fcbAnswers[0][3], "", fcbAnswers[2][3]},
		{"pau", "102", "AWAITING_SUBMISSION", "", "", ""},
	}
	if got := readCSV(t, buf.String()); !reflect.DeepEqual(got, want) {
		t.Errorf("csv =\n%v\nwant\n%v", got, want)
	}
}

// A module switched to prompts can still hold submissions written before the
// switch. They stay in the pivot, with their Markdown in a trailing column
// that exists only when such a row does.
func TestRenderTeacherAssignmentsCSV_WideKeepsLegacyWrittenRows(t *testing.T) {
	data := enrichedRows(
		assignmentRow("ana", "102", "SUBMITTED", promptsEvidence(fcbAnswers...)),
		assignmentRow("old", "102", "ACCEPTED", tiptapDoc("Written before the switch.")),
	)
	var buf bytes.Buffer
	if err := renderTeacherAssignmentsCSV(data, true, &buf); err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"student_alias", "course_module_code", "status", "c102-cause", "c102-value", "c102-ask", "evidence_text"},
		{"ana", "102", "SUBMITTED", fcbAnswers[0][3], fcbAnswers[1][3], fcbAnswers[2][3], ""},
		{"old", "102", "ACCEPTED", "", "", "", "Written before the switch."},
	}
	if got := readCSV(t, buf.String()); !reflect.DeepEqual(got, want) {
		t.Errorf("csv =\n%v\nwant\n%v", got, want)
	}
}

func TestRenderTeacherAssignmentsCSV_WideRefusesMixedOrNonPrompts(t *testing.T) {
	cases := map[string][]interface{}{
		"two modules": enrichedRows(
			assignmentRow("ana", "102", "SUBMITTED", promptsEvidence(fcbAnswers...)),
			assignmentRow("jordi", "101", "SUBMITTED", tiptapDoc("Written work.")),
		),
		"no prompts rows": enrichedRows(
			assignmentRow("jordi", "101", "SUBMITTED", tiptapDoc("Written work.")),
		),
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			err := renderTeacherAssignmentsCSV(data, true, &buf)
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), "--course <id> --module-code <code>") {
				t.Errorf("error = %q, want the --course/--module-code hint", err)
			}
			if buf.Len() != 0 {
				t.Errorf("wrote output before failing:\n%s", buf.String())
			}
		})
	}
}

func TestRenderTeacherAssignmentsMarkdown(t *testing.T) {
	data := enrichedRows(
		assignmentRow("ana", "102", "SUBMITTED", promptsEvidence(fcbAnswers...)),
		assignmentRow("pau", "102", "AWAITING_SUBMISSION", nil),
	)
	var buf bytes.Buffer
	if err := renderTeacherAssignmentsMarkdown(data, &buf); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	for _, want := range []string{
		"## ana · module 102", "Status: SUBMITTED", fcbEvidenceText,
		"## pau · module 102", "Status: AWAITING_SUBMISSION", "_No submission._",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("markdown missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "map[") {
		t.Errorf("markdown contains a Go map dump:\n%s", got)
	}

	buf.Reset()
	if err := renderTeacherAssignmentsMarkdown(nil, &buf); err != nil || buf.Len() != 0 {
		t.Errorf("empty markdown = %q, err %v; want nothing", buf.String(), err)
	}
}

func TestFilterRowsByModule(t *testing.T) {
	data := enrichedRows(
		assignmentRow("ana", "102", "SUBMITTED", nil),
		assignmentRow("jordi", "101", "SUBMITTED", nil),
	)
	got := filterRowsByModule(data, "102")
	if len(got) != 1 || got[0].(map[string]interface{})["student_alias"] != "ana" {
		t.Errorf("filtered = %v", got)
	}
}

// --- handler level -----------------------------------------------------------

const twoModuleBody = `{
    "data": [
        {"course_id": "C1", "course_module_code": "101", "student_alias": "jordi",
         "content": {"commitment_status": "SUBMITTED"}},
        {"course_id": "C1", "course_module_code": "102", "student_alias": "ana",
         "content": {"commitment_status": "SUBMITTED",
                     "evidence": {"type": "prompts-evidence", "version": 1, "answers": [
                        {"promptId": "c102-cause", "label": "The cause", "question": "Why?", "answer": "Water."}]}}}
    ],
    "meta": {"source": "merged"}
}`

// runListHandler drives the list handler with the given flags and format and
// returns stdout and the handler error. Flags are reset afterwards.
func runListHandler(t *testing.T, format string, flags map[string]string) (string, error) {
	t.Helper()
	cmd := teacherAssignmentsListCmd
	cmd.SetContext(context.Background())
	for name, value := range flags {
		if err := cmd.Flags().Set(name, value); err != nil {
			t.Fatalf("set --%s: %v", name, err)
		}
	}
	t.Cleanup(func() {
		_ = cmd.Flags().Set("course", "")
		_ = cmd.Flags().Set("module-code", "")
		_ = cmd.Flags().Set("wide", "false")
	})
	var runErr error
	captured := captureStdout(t, func() {
		_ = output.SetFormat(format)
		t.Cleanup(func() { _ = output.SetFormat("text") })
		runErr = cmd.RunE(cmd, []string{})
	})
	return captured, runErr
}

func TestRunTeacherAssignmentsList_ModuleFiltersJSON(t *testing.T) {
	teacherAssignmentsHandlerEnv(t, twoModuleBody)
	out, err := runListHandler(t, "json", map[string]string{"course": "C1", "module-code": "102"})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	data := got["data"].([]interface{})
	if len(data) != 1 || data[0].(map[string]interface{})["student_alias"] != "ana" {
		t.Errorf("data = %v, want only ana", data)
	}
	if got["meta"] == nil {
		t.Error("envelope keys beside data were dropped")
	}
}

func TestRunTeacherAssignmentsList_ModuleFiltersText(t *testing.T) {
	teacherAssignmentsHandlerEnv(t, twoModuleBody)
	out, err := runListHandler(t, "text", map[string]string{"course": "C1", "module-code": "101"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "jordi") || strings.Contains(out, "ana") {
		t.Errorf("text output not filtered to module 101:\n%s", out)
	}
}

func TestRunTeacherAssignmentsList_WideWithModuleCSV(t *testing.T) {
	teacherAssignmentsHandlerEnv(t, twoModuleBody)
	out, err := runListHandler(t, "csv", map[string]string{"course": "C1", "module-code": "102", "wide": "true"})
	if err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"student_alias", "course_module_code", "status", "c102-cause"},
		{"ana", "102", "SUBMITTED", "Water."},
	}
	if got := readCSV(t, out); !reflect.DeepEqual(got, want) {
		t.Errorf("csv = %v, want %v", got, want)
	}
}

func TestRunTeacherAssignmentsList_WideWithoutModuleFailsCleanly(t *testing.T) {
	teacherAssignmentsHandlerEnv(t, twoModuleBody)
	out, err := runListHandler(t, "csv", map[string]string{"course": "C1", "wide": "true"})
	if err == nil || !strings.Contains(err.Error(), "--module-code") {
		t.Fatalf("err = %v, want the --module-code hint", err)
	}
	if out != "" {
		t.Errorf("stdout = %q, want empty", out)
	}
}

// Flag misuse is rejected before any request reaches the gateway.
func TestRunTeacherAssignmentsList_FlagMisuseSendsNoRequest(t *testing.T) {
	cases := []struct {
		name   string
		format string
		flags  map[string]string
		want   string
	}{
		{"wide outside csv", "json", map[string]string{"course": "C1", "module-code": "102", "wide": "true"}, "--wide"},
		{"module without course", "text", map[string]string{"module-code": "102"}, "--course"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			requests := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				_, _ = w.Write([]byte(`{"data": []}`))
			}))
			t.Cleanup(srv.Close)
			teacherAssignmentsHandlerEnvURL(t, srv.URL)

			_, err := runListHandler(t, tc.format, tc.flags)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want mention of %s", err, tc.want)
			}
			if requests != 0 {
				t.Errorf("requests = %d, want 0", requests)
			}
		})
	}
}

func TestRunTeacherAssignmentsGet_CSV(t *testing.T) {
	teacherAssignmentsHandlerEnv(t, twoModuleBody)
	cmd := teacherAssignmentsGetCmd
	cmd.SetContext(context.Background())
	var runErr error
	out := captureStdout(t, func() {
		_ = output.SetFormat("csv")
		t.Cleanup(func() { _ = output.SetFormat("text") })
		runErr = cmd.RunE(cmd, []string{"C1", "102", "ana"})
	})
	if runErr != nil {
		t.Fatal(runErr)
	}
	want := [][]string{longHeader, {"ana", "102", "SUBMITTED", "c102-cause", "The cause", "Why?", "Water."}}
	if got := readCSV(t, out); !reflect.DeepEqual(got, want) {
		t.Errorf("csv = %v, want %v", got, want)
	}
}

// teacherAssignmentsHandlerEnvURL seeds config pointing at an existing server.
func teacherAssignmentsHandlerEnvURL(t *testing.T, url string) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	if err := config.Save(&config.Config{BaseURL: url}); err != nil {
		t.Fatalf("seed config: %v", err)
	}
}

// A degraded read (206 with meta.warning) must warn on stderr in CSV and
// Markdown, on both list and get, per the every-mode-except-JSON rule.
func TestTeacherAssignments_DegradedReadWarnsInCSVAndMarkdown(t *testing.T) {
	const warning = "DB API unavailable, showing on-chain data only"
	body := strings.Replace(twoModuleBody, `"meta": {"source": "merged"}`,
		`"meta": {"source": "merged", "warning": "`+warning+`"}`, 1)

	for _, format := range []string{"csv", "markdown"} {
		t.Run("list/"+format, func(t *testing.T) {
			teacherAssignmentsHandlerEnv(t, body)
			stderr := captureStderr(t, func() {
				if _, err := runListHandler(t, format, map[string]string{"course": "C1"}); err != nil {
					t.Fatal(err)
				}
			})
			if !strings.Contains(stderr, warning) {
				t.Errorf("stderr = %q, want the degraded-read warning", stderr)
			}
		})
		t.Run("get/"+format, func(t *testing.T) {
			teacherAssignmentsHandlerEnv(t, body)
			cmd := teacherAssignmentsGetCmd
			cmd.SetContext(context.Background())
			stderr := captureStderr(t, func() {
				captureStdout(t, func() {
					_ = output.SetFormat(format)
					t.Cleanup(func() { _ = output.SetFormat("text") })
					if err := cmd.RunE(cmd, []string{"C1", "102", "ana"}); err != nil {
						t.Fatal(err)
					}
				})
			})
			if !strings.Contains(stderr, warning) {
				t.Errorf("stderr = %q, want the degraded-read warning", stderr)
			}
		})
	}
}

// "No submission" is a factual claim. It is made only when the row carries a
// content object with no evidence; evidence the CLI cannot render, and summary
// rows that carry no content at all, say so instead.
func TestRenderTeacherAssignmentsMarkdown_OnlyClaimsNoSubmissionWhenThereIsNone(t *testing.T) {
	quizEvidence := map[string]interface{}{"type": "quiz-evidence", "version": float64(1), "answers": []interface{}{}}
	cases := []struct {
		name    string
		row     map[string]interface{}
		want    string
		notWant string
	}{
		{"no evidence", assignmentRow("pau", "102", "AWAITING_SUBMISSION", nil), "_No submission._", ""},
		{"quiz evidence", assignmentRow("quin", "103", "SUBMITTED", quizEvidence), "--output json", "_No submission._"},
		{"summary row", map[string]interface{}{"course_id": "C1", "course_module_code": "102", "student_alias": "ana"}, "--course", "_No submission._"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := renderTeacherAssignmentsMarkdown(enrichedRows(tc.row), &buf); err != nil {
				t.Fatal(err)
			}
			got := buf.String()
			if !strings.Contains(got, tc.want) {
				t.Errorf("markdown missing %q:\n%s", tc.want, got)
			}
			if tc.notWant != "" && strings.Contains(got, tc.notWant) {
				t.Errorf("markdown contains %q:\n%s", tc.notWant, got)
			}
		})
	}
}

// Spreadsheet apps run a cell that starts with =, +, -, @, tab or carriage
// return as a formula. Learner answers are untrusted, so such cells are
// written with a leading single quote, which the apps treat as "text".
func TestRenderTeacherAssignmentsCSV_NeutralizesFormulaCells(t *testing.T) {
	payloads := []string{"=HYPERLINK(\"http://x\",\"y\")", "+1+1", "-2+3", "@SUM(A1)", "\t=1", "\r=1"}
	answers := make([][4]string, 0, len(payloads))
	for i, p := range payloads {
		answers = append(answers, [4]string{fmt.Sprintf("p%d", i), "Label", "Q?", p})
	}
	data := enrichedRows(
		assignmentRow("ana", "102", "SUBMITTED", promptsEvidence(answers...)),
		assignmentRow("jordi", "102", "SUBMITTED", tiptapDoc("=cmd|' /C calc'!A0")),
	)

	for _, wide := range []bool{false, true} {
		var buf bytes.Buffer
		if err := renderTeacherAssignmentsCSV(data, wide, &buf); err != nil {
			t.Fatalf("wide=%v: %v", wide, err)
		}
		for _, record := range readCSV(t, buf.String())[1:] {
			for _, cell := range record {
				if cell != "" && strings.ContainsRune("=+-@\t\r", rune(cell[0])) {
					t.Errorf("wide=%v: cell %q starts with a formula character", wide, cell)
				}
			}
		}
	}

	var buf bytes.Buffer
	_ = renderTeacherAssignmentsCSV(data, false, &buf)
	if got := readCSV(t, buf.String())[1][6]; got != "'"+payloads[0] {
		t.Errorf("answer = %q, want the payload prefixed with a single quote", got)
	}
}

// Ordinary text, including text with a formula character after the first
// position, is written unchanged.
func TestRenderTeacherAssignmentsCSV_LeavesOrdinaryCellsAlone(t *testing.T) {
	data := enrichedRows(assignmentRow("ana", "102", "SUBMITTED",
		promptsEvidence([4]string{"c102-cause", "The cause", "Why?", "Water = life, +1 for that"})))
	var buf bytes.Buffer
	if err := renderTeacherAssignmentsCSV(data, false, &buf); err != nil {
		t.Fatal(err)
	}
	if got := readCSV(t, buf.String())[1]; got[3] != "c102-cause" || got[6] != "Water = life, +1 for that" {
		t.Errorf("row = %v", got)
	}
}

// Prompt ids come from the learner's evidence, which is opaque JSON a learner
// can submit directly, so the --wide header cells built from them are
// untrusted and get the same formula guard as data cells.
func TestRenderTeacherAssignmentsCSV_WideGuardsHeaderFromEvidence(t *testing.T) {
	data := enrichedRows(assignmentRow("ana", "102", "SUBMITTED", promptsEvidence(
		[4]string{`=HYPERLINK("http://evil","click")`, "L", "Q?", "answer"},
	)))
	var buf bytes.Buffer
	if err := renderTeacherAssignmentsCSV(data, true, &buf); err != nil {
		t.Fatal(err)
	}
	header := readCSV(t, buf.String())[0]
	if got := header[3]; got != `'=HYPERLINK("http://evil","click")` {
		t.Errorf("header cell = %q, want it prefixed with a single quote", got)
	}
}

// A prompt id equal to a base column would duplicate that header, and readers
// that key by header name silently overwrite one column with the other. An
// empty id would produce a nameless column. --wide refuses both.
func TestRenderTeacherAssignmentsCSV_WideRefusesCollidingPromptIDs(t *testing.T) {
	for _, id := range []string{"student_alias", "course_module_code", "status", "evidence_text", ""} {
		t.Run(fmt.Sprintf("%q", id), func(t *testing.T) {
			data := enrichedRows(
				assignmentRow("ana", "102", "SUBMITTED", promptsEvidence(fcbAnswers...)),
				assignmentRow("mallory", "102", "SUBMITTED", promptsEvidence([4]string{id, "L", "Q?", "x"})),
			)
			var buf bytes.Buffer
			err := renderTeacherAssignmentsCSV(data, true, &buf)
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), "mallory") || !strings.Contains(err.Error(), "without --wide") {
				t.Errorf("error = %q, want the student named and the long-form way out", err)
			}
			if buf.Len() != 0 {
				t.Errorf("wrote output before failing:\n%s", buf.String())
			}
		})
	}
}

// Some importers (LibreOffice with "Trim spaces") strip leading spaces before
// deciding a cell is a formula, so the guard looks past them.
func TestNeutralizeCSVFormula_LooksPastLeadingSpaces(t *testing.T) {
	cases := map[string]string{
		" =1+1":      "' =1+1",
		"   @SUM(A)": "'   @SUM(A)",
		"  plain":    "  plain",
		"a =b":       "a =b",
		"":           "",
		"   ":        "   ",
	}
	for in, want := range cases {
		if got := neutralizeCSVFormula(in); got != want {
			t.Errorf("neutralizeCSVFormula(%q) = %q, want %q", in, got, want)
		}
	}
}
