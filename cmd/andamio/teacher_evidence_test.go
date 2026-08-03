package main

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

// tiptapDoc builds a Tiptap document with a single paragraph of plain text.
func tiptapDoc(text string) map[string]interface{} {
	return map[string]interface{}{
		"type": "doc",
		"content": []interface{}{
			map[string]interface{}{
				"type": "paragraph",
				"content": []interface{}{
					map[string]interface{}{"type": "text", "text": text},
				},
			},
		},
	}
}

func commitmentRow(evidence interface{}) map[string]interface{} {
	content := map[string]interface{}{"commitment_status": "SUBMITTED"}
	if evidence != nil {
		content["evidence"] = evidence
	}
	return map[string]interface{}{
		"student_alias":      "student-01",
		"course_module_code": "101",
		"content":            content,
	}
}

func contentOf(t *testing.T, row map[string]interface{}) map[string]interface{} {
	t.Helper()
	content, ok := row["content"].(map[string]interface{})
	if !ok {
		t.Fatal("row has no content object")
	}
	return content
}

func TestEnrichCommitmentEvidence_DecodesProse(t *testing.T) {
	row := commitmentRow(tiptapDoc("My evidence submission"))
	enrichCommitmentEvidence(row)

	got, ok := contentOf(t, row)[evidenceTextField].(string)
	if !ok {
		t.Fatalf("evidence_text absent or not a string: %v", contentOf(t, row)[evidenceTextField])
	}
	if got != "My evidence submission" {
		t.Errorf("evidence_text = %q, want %q", got, "My evidence submission")
	}
}

// The hash-bearing form must survive untouched. Comparing a pre-enrichment
// deep copy catches in-place mutation anywhere in the tree, not just at the
// top level.
func TestEnrichCommitmentEvidence_LeavesRawEvidenceUntouched(t *testing.T) {
	original := tiptapDoc("My evidence submission")

	marshalled, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var before map[string]interface{}
	if err := json.Unmarshal(marshalled, &before); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	row := commitmentRow(original)
	enrichCommitmentEvidence(row)

	after, ok := contentOf(t, row)["evidence"].(map[string]interface{})
	if !ok {
		t.Fatal("evidence was removed or retyped")
	}
	if !reflect.DeepEqual(before, after) {
		t.Errorf("evidence was mutated:\nbefore: %v\nafter:  %v", before, after)
	}
}

// Richer Tiptap must decode via the same converter course export uses, rather
// than through a second, drift-prone traversal.
func TestEnrichCommitmentEvidence_MatchesExportConverter(t *testing.T) {
	doc := map[string]interface{}{
		"type": "doc",
		"content": []interface{}{
			map[string]interface{}{
				"type":  "heading",
				"attrs": map[string]interface{}{"level": float64(2)},
				"content": []interface{}{
					map[string]interface{}{"type": "text", "text": "Approach"},
				},
			},
			map[string]interface{}{
				"type": "paragraph",
				"content": []interface{}{
					map[string]interface{}{
						"type":  "text",
						"text":  "bold claim",
						"marks": []interface{}{map[string]interface{}{"type": "bold"}},
					},
				},
			},
			map[string]interface{}{
				"type": "bulletList",
				"content": []interface{}{
					map[string]interface{}{
						"type": "listItem",
						"content": []interface{}{
							map[string]interface{}{
								"type": "paragraph",
								"content": []interface{}{
									map[string]interface{}{"type": "text", "text": "first"},
								},
							},
						},
					},
				},
			},
		},
	}

	want, _ := tiptapToMarkdown(doc)
	want = strings.TrimSpace(want)

	row := commitmentRow(doc)
	enrichCommitmentEvidence(row)

	got, _ := contentOf(t, row)[evidenceTextField].(string)
	if got != want {
		t.Errorf("evidence_text = %q, want %q (must match the export converter)", got, want)
	}
	for _, fragment := range []string{"Approach", "bold claim", "first"} {
		if !strings.Contains(got, fragment) {
			t.Errorf("decoded evidence lost %q: %q", fragment, got)
		}
	}
}

// Absence, not empty string — a caller branching on presence must not be
// handed "" for "no submission".
func TestEnrichCommitmentEvidence_AbsentRatherThanEmpty(t *testing.T) {
	cases := []struct {
		name     string
		evidence interface{}
	}{
		{"no evidence key", nil},
		{"evidence is a string", "https://github.com/user/repo"},
		{"evidence is an empty doc", map[string]interface{}{"type": "doc"}},
		{"evidence is a whitespace-only doc", tiptapDoc("   ")},
		{"evidence is a number", float64(42)},
		{"evidence is nil", map[string]interface{}(nil)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			row := commitmentRow(tc.evidence)
			enrichCommitmentEvidence(row)

			if _, present := contentOf(t, row)[evidenceTextField]; present {
				t.Errorf("evidence_text was set for %s; it must be absent", tc.name)
			}
		})
	}
}

// The no---course summary shape has no nested content object. It must pass
// through untouched, exactly as it already does for commitment_status.
func TestEnrichCommitmentEvidence_SummaryShapeUntouched(t *testing.T) {
	row := map[string]interface{}{
		"student_alias":      "student-01",
		"course_module_code": "101",
		"source":             "chain_only",
	}
	before, _ := json.Marshal(row)

	enrichCommitmentEvidence(row)

	after, _ := json.Marshal(row)
	if string(before) != string(after) {
		t.Errorf("summary-shape row was modified:\nbefore: %s\nafter:  %s", before, after)
	}
}

func TestEnrichCommitmentRows_EnrichesEveryRow(t *testing.T) {
	resp := map[string]interface{}{
		"data": []interface{}{
			commitmentRow(tiptapDoc("first submission")),
			commitmentRow(tiptapDoc("second submission")),
			map[string]interface{}{"student_alias": "student-03"}, // summary shape
		},
	}

	enrichCommitmentRows(resp)

	data := resp["data"].([]interface{})
	for i, want := range []string{"first submission", "second submission"} {
		row := data[i].(map[string]interface{})
		got, _ := contentOf(t, row)[evidenceTextField].(string)
		if got != want {
			t.Errorf("row %d evidence_text = %q, want %q", i, got, want)
		}
	}
	if _, hasContent := data[2].(map[string]interface{})["content"]; hasContent {
		t.Error("summary-shape row gained a content object")
	}
}

// The contract issue #124 actually specifies, exercised over the wire: an
// agent lists commitments awaiting review and reads what was submitted,
// without implementing Tiptap traversal.
func TestFetchTeacherAssignmentsList_DecodesEvidenceOverTheWire(t *testing.T) {
	wire := map[string]interface{}{
		"data": []interface{}{
			map[string]interface{}{
				"student_alias":      "student-01",
				"course_module_code": "101",
				"content": map[string]interface{}{
					"commitment_status": "SUBMITTED",
					"evidence":          tiptapDoc("This course is about the programming language, Go."),
					"evidence_hash":     "deadbeef",
				},
			},
		},
	}

	_, c := stubTeacherAssignmentsServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(wire)
	})

	resp, err := fetchTeacherAssignmentsList(t.Context(), c, "course-1")
	if err != nil {
		t.Fatalf("fetchTeacherAssignmentsList: %v", err)
	}

	row := resp["data"].([]interface{})[0].(map[string]interface{})
	content := contentOf(t, row)

	got, _ := content[evidenceTextField].(string)
	if got != "This course is about the programming language, Go." {
		t.Errorf("evidence_text = %q", got)
	}
	if _, ok := content["evidence"].(map[string]interface{}); !ok {
		t.Error("raw evidence document did not survive the round trip")
	}
	if content["evidence_hash"] != "deadbeef" {
		t.Errorf("evidence_hash was dropped: %v", content["evidence_hash"])
	}
	if content["commitment_status"] != "SUBMITTED" {
		t.Errorf("commitment_status was dropped: %v", content["commitment_status"])
	}
}

// Empty result sets are the common case for a course with nothing awaiting
// review; enrichment must not turn that into a crash.
func TestFetchTeacherAssignmentsList_EmptyResultIsNotAnError(t *testing.T) {
	_, c := stubTeacherAssignmentsServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[]}`))
	})

	resp, err := fetchTeacherAssignmentsList(t.Context(), c, "course-1")
	if err != nil {
		t.Fatalf("empty result returned an error: %v", err)
	}
	data, ok := resp["data"].([]interface{})
	if !ok || len(data) != 0 {
		t.Errorf("data = %v, want an empty array", resp["data"])
	}
}

// Shapes that would panic a naive walker. The empty-data case is the one that
// actually occurs: a course with no commitments yet.
func TestEnrichCommitmentRows_MalformedEnvelopes(t *testing.T) {
	cases := []struct {
		name string
		resp map[string]interface{}
	}{
		{"empty data array", map[string]interface{}{"data": []interface{}{}}},
		{"no data key", map[string]interface{}{}},
		{"data is not an array", map[string]interface{}{"data": "unexpected"}},
		{"data is null", map[string]interface{}{"data": nil}},
		{"row is not an object", map[string]interface{}{"data": []interface{}{"unexpected"}}},
		{"row is null", map[string]interface{}{"data": []interface{}{nil}}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			enrichCommitmentRows(tc.resp) // must not panic
		})
	}
}
