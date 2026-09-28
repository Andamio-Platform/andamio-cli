package main

import (
	"encoding/json"
	"net/http"
	"reflect"
	"testing"

	"github.com/Andamio-Platform/andamio-cli/internal/prompts"
)

// promptsEvidence builds a prompts-evidence envelope as the fcb app writes it
// (camelCase promptId), decoded the way the gateway response decodes.
func promptsEvidence(answers ...[4]string) map[string]interface{} {
	list := make([]interface{}, 0, len(answers))
	for _, a := range answers {
		list = append(list, map[string]interface{}{
			"promptId": a[0], "label": a[1], "question": a[2], "answer": a[3],
		})
	}
	return map[string]interface{}{"type": "prompts-evidence", "version": float64(1), "answers": list}
}

var fcbAnswers = [][4]string{
	{"c102-cause", "The cause", "What should people see, and why does it matter to you?", "Clean water for Raval schools."},
	{"c102-value", "The value", "Which club value does it connect to?", "It's my neighborhood."},
	{"c102-ask", "The ask", "What one thing would you ask?", "Match fan donations."},
}

const fcbEvidenceText = "**The cause.** What should people see, and why does it matter to you?\n" +
	"Clean water for Raval schools.\n\n" +
	"**The value.** Which club value does it connect to?\n" +
	"It's my neighborhood.\n\n" +
	"**The ask.** What one thing would you ask?\n" +
	"Match fan donations."

func deepCopyJSON(t *testing.T, v interface{}) interface{} {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var out interface{}
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestEnrichCommitmentEvidence_PromptsEvidence(t *testing.T) {
	evidence := promptsEvidence(fcbAnswers...)
	before := deepCopyJSON(t, evidence)

	row := commitmentRow(evidence)
	enrichCommitmentEvidence(row)
	content := contentOf(t, row)

	if got, _ := content[evidenceTextField].(string); got != fcbEvidenceText {
		t.Errorf("evidence_text =\n%q\nwant\n%q", got, fcbEvidenceText)
	}

	answers, ok := content[evidenceAnswersField].([]prompts.Answer)
	if !ok {
		t.Fatalf("evidence_answers absent or wrong type: %T", content[evidenceAnswersField])
	}
	if len(answers) != 3 || answers[0].PromptID != "c102-cause" || answers[2].Answer != "Match fan donations." {
		t.Errorf("evidence_answers = %+v", answers)
	}

	// The JSON a script sees uses snake_case keys.
	out, _ := json.Marshal(content[evidenceAnswersField])
	var decoded []map[string]interface{}
	_ = json.Unmarshal(out, &decoded)
	if decoded[0]["prompt_id"] != "c102-cause" || decoded[0]["promptId"] != nil {
		t.Errorf("evidence_answers JSON = %s, want prompt_id keys", out)
	}

	if after := deepCopyJSON(t, content["evidence"]); !reflect.DeepEqual(before, after) {
		t.Errorf("evidence was mutated:\nbefore: %v\nafter:  %v", before, after)
	}
}

func TestEnrichCommitmentEvidence_PromptsEvidenceAbsentRatherThanEmpty(t *testing.T) {
	cases := map[string]interface{}{
		"no answers": promptsEvidence(),
		"malformed answer": map[string]interface{}{
			"type": "prompts-evidence", "version": float64(1),
			"answers": []interface{}{map[string]interface{}{"promptId": "a", "answer": float64(42)}},
		},
		"quiz evidence": map[string]interface{}{"type": "quiz-evidence", "version": float64(1), "answers": []interface{}{}},
	}
	for name, evidence := range cases {
		t.Run(name, func(t *testing.T) {
			row := commitmentRow(evidence)
			enrichCommitmentEvidence(row)
			content := contentOf(t, row)
			if _, present := content[evidenceAnswersField]; present {
				t.Errorf("evidence_answers present: %v", content[evidenceAnswersField])
			}
			if _, present := content[evidenceTextField]; present {
				t.Errorf("evidence_text present: %q", content[evidenceTextField])
			}
		})
	}
}

func TestEnrichCommitmentEvidence_TiptapGetsNoAnswers(t *testing.T) {
	row := commitmentRow(tiptapDoc("Written work."))
	enrichCommitmentEvidence(row)
	content := contentOf(t, row)
	if content[evidenceTextField] != "Written work." {
		t.Errorf("evidence_text = %v", content[evidenceTextField])
	}
	if _, present := content[evidenceAnswersField]; present {
		t.Error("Tiptap evidence gained evidence_answers")
	}
}

func TestFetchTeacherAssignmentsList_DecodesPromptsEvidenceOverTheWire(t *testing.T) {
	wire := map[string]interface{}{
		"data": []interface{}{
			map[string]interface{}{
				"student_alias": "ana", "course_module_code": "102",
				"content": map[string]interface{}{"commitment_status": "SUBMITTED", "evidence": promptsEvidence(fcbAnswers...)},
			},
			map[string]interface{}{
				"student_alias": "jordi", "course_module_code": "101",
				"content": map[string]interface{}{"commitment_status": "SUBMITTED", "evidence": tiptapDoc("Written work.")},
			},
		},
	}
	_, c := stubTeacherAssignmentsServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(wire)
	})

	resp, err := fetchTeacherAssignmentsList(t.Context(), c, "course-1")
	if err != nil {
		t.Fatal(err)
	}
	data := resp["data"].([]interface{})
	ana := contentOf(t, data[0].(map[string]interface{}))
	jordi := contentOf(t, data[1].(map[string]interface{}))

	if ana[evidenceTextField] != fcbEvidenceText {
		t.Errorf("ana evidence_text = %q", ana[evidenceTextField])
	}
	if _, ok := ana[evidenceAnswersField].([]prompts.Answer); !ok {
		t.Error("ana has no evidence_answers")
	}
	if jordi[evidenceTextField] != "Written work." {
		t.Errorf("jordi evidence_text = %q", jordi[evidenceTextField])
	}
	if _, present := jordi[evidenceAnswersField]; present {
		t.Error("jordi gained evidence_answers")
	}
}
