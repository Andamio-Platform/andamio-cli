package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/Andamio-Platform/andamio-cli/internal/prompts"
)

// CSV and Markdown renderers for `teacher assignments list|get` (cli#171).
// The generic output paths print nested maps with %v, which turns a
// submission into a Go map dump; these read the enriched rows instead. JSON
// output does not come through here: it passes the gateway envelope through.

// commitmentView is the part of an enriched commitment row the renderers read.
type commitmentView struct {
	courseID string
	alias    string
	module   string
	status   string
	answers  []prompts.Answer // prompts evidence; nil otherwise
	text     string           // evidence_text; empty when absent

	hasContent  bool // the row carries a content object (absent on the no-course summary)
	hasEvidence bool // content.evidence is present, whether or not the CLI could render it
}

func viewOfCommitment(row map[string]interface{}) commitmentView {
	v := commitmentView{}
	v.courseID, _ = row["course_id"].(string)
	v.alias, _ = row["student_alias"].(string)
	v.module, _ = row["course_module_code"].(string)
	if content, ok := row["content"].(map[string]interface{}); ok {
		v.hasContent = true
		v.hasEvidence = content["evidence"] != nil
		v.status, _ = content["commitment_status"].(string)
		v.answers, _ = content[evidenceAnswersField].([]prompts.Answer)
		v.text, _ = content[evidenceTextField].(string)
	}
	return v
}

func viewsOf(data []interface{}) []commitmentView {
	views := make([]commitmentView, 0, len(data))
	for _, item := range data {
		if row, ok := item.(map[string]interface{}); ok {
			views = append(views, viewOfCommitment(row))
		}
	}
	return views
}

// filterRowsByModule keeps the rows whose course_module_code equals code.
func filterRowsByModule(data []interface{}, code string) []interface{} {
	out := make([]interface{}, 0, len(data))
	for _, item := range data {
		if row, ok := item.(map[string]interface{}); ok && row["course_module_code"] == code {
			out = append(out, item)
		}
	}
	return out
}

var assignmentsCSVBaseHeader = []string{"student_alias", "course_module_code", "status"}

// renderTeacherAssignmentsCSV writes one row per prompts answer by default. A
// commitment without prompts evidence is one row with blank prompt columns and
// its Markdown, if any, in answer. wide pivots to one row per commitment; see
// wideAssignmentsCSV.
func renderTeacherAssignmentsCSV(data []interface{}, wide bool, w io.Writer) error {
	views := viewsOf(data)
	var records [][]string
	if wide {
		var err error
		if records, err = wideAssignmentsCSV(views); err != nil {
			return err
		}
	} else {
		records = longAssignmentsCSV(views)
	}
	// Every cell, header included: the --wide header is built from prompt ids,
	// which come from learner-submitted evidence.
	for _, record := range records {
		for i, cell := range record {
			record[i] = neutralizeCSVFormula(cell)
		}
	}
	cw := csv.NewWriter(w)
	if err := cw.WriteAll(records); err != nil {
		return err
	}
	return cw.Error()
}

// neutralizeCSVFormula prefixes a cell with a single quote when it starts with
// a character spreadsheet apps read as the start of a formula (=, +, -, @, tab,
// carriage return). Answers are learner-typed, and this CSV exists to be opened
// in Excel or Sheets, where "=HYPERLINK(...)" would otherwise run. The quote
// makes the apps treat the cell as text. Leading spaces are looked past,
// because some importers (LibreOffice with "Trim spaces") strip them before
// deciding.
func neutralizeCSVFormula(cell string) string {
	trimmed := strings.TrimLeft(cell, " ")
	if trimmed != "" && strings.ContainsRune("=+-@\t\r", rune(trimmed[0])) {
		return "'" + cell
	}
	return cell
}

func longAssignmentsCSV(views []commitmentView) [][]string {
	header := slices.Concat(assignmentsCSVBaseHeader, []string{"prompt_id", "label", "question", "answer"})
	records := [][]string{header}
	for _, v := range views {
		if len(v.answers) == 0 {
			records = append(records, []string{v.alias, v.module, v.status, "", "", "", v.text})
			continue
		}
		for _, a := range v.answers {
			records = append(records, []string{v.alias, v.module, v.status, a.PromptID, a.Label, a.Question, a.Answer})
		}
	}
	return records
}

// wideAssignmentsCSV writes one row per commitment and one column per prompt
// id, in first-seen order. It needs every row from one course and module and
// at least one prompts submission; otherwise the columns would not mean the
// same thing on every row. A row in that module with written (Tiptap)
// evidence, submitted before the module switched to prompts, keeps blank
// prompt cells and gets its Markdown in a trailing evidence_text column, which
// exists only when such a row does. Prompt ids come from learner-submitted
// evidence, so an empty id or one that would duplicate a fixed column is
// refused rather than written. All checks run before anything is written.
func wideAssignmentsCSV(views []commitmentView) ([][]string, error) {
	if len(views) == 0 {
		return [][]string{assignmentsCSVBaseHeader}, nil
	}

	hint := "pass --course <id> --module <code> to pick one prompts module"
	first := views[0]
	var ids []string
	seen := map[string]bool{}
	written := false
	for _, v := range views {
		if v.courseID != first.courseID || v.module != first.module {
			return nil, fmt.Errorf("--wide needs every row from one prompts module, but this result spans more than one module; %s", hint)
		}
		for _, a := range v.answers {
			if a.PromptID == "" || slices.Contains(assignmentsCSVBaseHeader, a.PromptID) || a.PromptID == evidenceTextField {
				return nil, fmt.Errorf("--wide cannot use prompt id %q from %s's submission as a column name: it is empty or matches a fixed column. Run without --wide for one row per answer", a.PromptID, v.alias)
			}
			if !seen[a.PromptID] {
				seen[a.PromptID] = true
				ids = append(ids, a.PromptID)
			}
		}
		if len(v.answers) == 0 && v.text != "" {
			written = true
		}
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("--wide needs a prompts assignment, but module %s has no prompts submissions; %s", first.module, hint)
	}

	header := slices.Concat(assignmentsCSVBaseHeader, ids)
	if written {
		header = append(header, evidenceTextField)
	}
	records := [][]string{header}
	for _, v := range views {
		byID := make(map[string]string, len(v.answers))
		for _, a := range v.answers {
			byID[a.PromptID] = a.Answer
		}
		record := []string{v.alias, v.module, v.status}
		for _, id := range ids {
			record = append(record, byID[id])
		}
		if written {
			if len(v.answers) == 0 {
				record = append(record, v.text)
			} else {
				record = append(record, "")
			}
		}
		records = append(records, record)
	}
	return records, nil
}

// renderTeacherAssignmentsMarkdown writes one section per commitment: the
// student and module as a heading, the status, then the submission.
func renderTeacherAssignmentsMarkdown(data []interface{}, w io.Writer) error {
	var b strings.Builder
	for i, v := range viewsOf(data) {
		if i > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "## %s · module %s\n\n", v.alias, v.module)
		if v.status != "" {
			fmt.Fprintf(&b, "Status: %s\n\n", v.status)
		}
		switch {
		case v.text != "":
			b.WriteString(v.text + "\n")
		case v.hasEvidence:
			b.WriteString("_Submission not rendered here. Read it with --output json (.content.evidence)._\n")
		case v.hasContent:
			b.WriteString("_No submission._\n")
		default:
			b.WriteString("_Submission not included in the summary. Pass --course <id> to read it._\n")
		}
	}
	_, err := io.WriteString(w, b.String())
	return err
}
