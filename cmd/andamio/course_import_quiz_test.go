package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Andamio-Platform/andamio-cli/internal/config"
)

const quizOutlineMD = "---\ntitle: Module 101\ncode: \"101\"\n---\n\n## SLTs\n\n1. Do a thing\n"

// writeQuizModuleDir builds a minimal compiled module directory with the given
// assignment files. quizJSON nil means no assignment.quiz.json; withMD adds an
// assignment.md.
func writeQuizModuleDir(t *testing.T, quizJSON []byte, withMD bool) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "outline.md"), []byte(quizOutlineMD), 0644); err != nil {
		t.Fatal(err)
	}
	if quizJSON != nil {
		if err := os.WriteFile(filepath.Join(dir, "assignment.quiz.json"), quizJSON, 0644); err != nil {
			t.Fatal(err)
		}
	}
	if withMD {
		if err := os.WriteFile(filepath.Join(dir, "assignment.md"), []byte("# Essay\n\nWrite it.\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func prettyQuiz(t *testing.T) []byte {
	t.Helper()
	b, err := json.MarshalIndent(quizEnvelope(), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return append(b, '\n')
}

// payloadAssignment decodes the dry-run payload's assignment through JSON so
// RawMessage and map values compare structurally (KTD3).
func payloadAssignment(t *testing.T, resp map[string]interface{}) map[string]interface{} {
	t.Helper()
	b, err := json.Marshal(resp["payload"])
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(b, &payload); err != nil {
		t.Fatal(err)
	}
	assign, ok := payload["assignment"].(map[string]interface{})
	if !ok {
		t.Fatalf("payload has no assignment object: %v", payload)
	}
	return assign
}

func TestReadCompiledModule_QuizAssignment(t *testing.T) {
	raw := prettyQuiz(t)
	dir := writeQuizModuleDir(t, raw, false)

	data, err := readCompiledModule(dir)
	if err != nil {
		t.Fatalf("readCompiledModule: %v", err)
	}
	if data.Assignment == nil {
		t.Fatal("Assignment is nil for a directory with assignment.quiz.json")
	}
	if string(data.Assignment.RawJSON) != string(raw) {
		t.Errorf("RawJSON differs from the file bytes")
	}
	if data.Assignment.Title != "" {
		t.Errorf("Title = %q, want empty so the existing title is preserved on import", data.Assignment.Title)
	}
	if data.Assignment.TiptapJSON != nil {
		t.Error("TiptapJSON must be nil for a quiz assignment")
	}
	if data.AssignmentQuiz == nil {
		t.Fatal("AssignmentQuiz summary is nil")
	}
	if data.AssignmentQuiz.QuestionCount != 2 || data.AssignmentQuiz.PassThreshold != 2 || !reflect.DeepEqual(data.AssignmentQuiz.QuestionIDs, []string{"q1", "q2"}) {
		t.Errorf("summary = %+v, want 2 questions, threshold 2, ids q1 q2", *data.AssignmentQuiz)
	}
}

// Both files present is never resolved by picking one (R11).
func TestReadCompiledModule_BothAssignmentFilesIsError(t *testing.T) {
	dir := writeQuizModuleDir(t, prettyQuiz(t), true)
	_, err := readCompiledModule(dir)
	if err == nil {
		t.Fatal("expected an error when both assignment.md and assignment.quiz.json exist")
	}
	if !strings.Contains(err.Error(), "assignment.md") || !strings.Contains(err.Error(), "assignment.quiz.json") {
		t.Errorf("error should name both files: %v", err)
	}
}

func TestReadCompiledModule_InvalidQuizListsEveryIssue(t *testing.T) {
	broken := quizEnvelope()
	broken["passThreshold"] = float64(9)
	broken["questions"].([]interface{})[1].(map[string]interface{})["correctValue"] = "zzz"
	raw, _ := json.Marshal(broken)
	dir := writeQuizModuleDir(t, raw, false)

	_, err := readCompiledModule(dir)
	if err == nil {
		t.Fatal("expected validation error")
	}
	msg := err.Error()
	for _, want := range []string{"assignment.quiz.json", "threshold-exceeds-questions", "dangling-correct-value", "q2"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q should mention %q", msg, want)
		}
	}
}

func TestReadCompiledModule_DocInQuizSlotIsError(t *testing.T) {
	dir := writeQuizModuleDir(t, []byte(`{"type":"doc","content":[]}`), false)
	_, err := readCompiledModule(dir)
	if err == nil || !strings.Contains(err.Error(), "assignment.md") {
		t.Fatalf("a Tiptap doc in assignment.quiz.json should be refused with a pointer at assignment.md, got %v", err)
	}
}

func TestReadCompiledModule_NonQuizTypeInQuizSlotIsError(t *testing.T) {
	dir := writeQuizModuleDir(t, []byte(`{"type":"quiz-evidence","version":1}`), false)
	if _, err := readCompiledModule(dir); err == nil {
		t.Fatal("a non-quiz envelope in assignment.quiz.json should be refused")
	}
}

func TestUpdateModuleContent_QuizPayloadPreservesMetadata(t *testing.T) {
	raw := prettyQuiz(t)
	data, err := readCompiledModule(writeQuizModuleDir(t, raw, false))
	if err != nil {
		t.Fatal(err)
	}
	existing := &ExistingModuleData{
		Status:   "ON_CHAIN",
		SLTCount: 1,
		Lessons:  map[int]map[string]interface{}{},
		Assignment: map[string]interface{}{
			"title":       "Module Quiz",
			"description": "Pass to continue",
			"image_url":   "https://cdn/x.png",
			"content_json": map[string]interface{}{
				"type": "doc", "content": []interface{}{},
			},
		},
	}

	resp, err := updateModuleContent(context.Background(), nil, "course-1", data, existing, true, true, false)
	if err != nil {
		t.Fatalf("updateModuleContent: %v", err)
	}
	assign := payloadAssignment(t, resp)
	if !reflect.DeepEqual(assign["content_json"], quizEnvelope()) {
		t.Errorf("content_json differs from the quiz file\n got: %v", assign["content_json"])
	}
	if assign["title"] != "Module Quiz" || assign["description"] != "Pass to continue" || assign["image_url"] != "https://cdn/x.png" {
		t.Errorf("existing metadata not preserved: %v", assign)
	}
	payload := resp["payload"].(map[string]interface{})
	if _, ok := payload["lessons"]; ok {
		t.Error("payload must not carry lessons when the directory has none")
	}
}

// The property #59 asks for: export then import of a quiz module produces a
// payload whose assignment is what the server already holds.
func TestExportImportRoundTrip_QuizIsServerNoOp(t *testing.T) {
	dir := t.TempDir()
	quiz := quizEnvelope()
	if _, err := writeCompiledModule(dir, exportModuleData(wrapAssignment(quiz, "Module Quiz"))); err != nil {
		t.Fatal(err)
	}

	data, err := readCompiledModule(dir)
	if err != nil {
		t.Fatalf("readCompiledModule after export: %v", err)
	}
	existing := &ExistingModuleData{
		Status:     "ON_CHAIN",
		SLTCount:   1,
		Lessons:    map[int]map[string]interface{}{},
		Assignment: map[string]interface{}{"title": "Module Quiz", "content_json": quiz},
	}
	resp, err := updateModuleContent(context.Background(), nil, "course-1", data, existing, true, true, false)
	if err != nil {
		t.Fatal(err)
	}
	assign := payloadAssignment(t, resp)
	if !reflect.DeepEqual(assign["content_json"], quiz) {
		t.Errorf("round trip changed the quiz\n got: %v\nwant: %v", assign["content_json"], quiz)
	}
	if assign["title"] != "Module Quiz" {
		t.Errorf("title = %v, want the existing title", assign["title"])
	}
}

// Markdown-only directories must produce the same payload as before: the quiz
// path is additive.
func TestUpdateModuleContent_MarkdownAssignmentUnchanged(t *testing.T) {
	dir := writeQuizModuleDir(t, nil, true)
	data, err := readCompiledModule(dir)
	if err != nil {
		t.Fatal(err)
	}
	if data.AssignmentQuiz != nil {
		t.Error("AssignmentQuiz must be nil for a Markdown assignment")
	}
	resp, err := updateModuleContent(context.Background(), nil, "course-1", data, &ExistingModuleData{Status: "DRAFT", Lessons: map[int]map[string]interface{}{}}, false, true, false)
	if err != nil {
		t.Fatal(err)
	}
	assign := payloadAssignment(t, resp)
	if assign["title"] != "Essay" {
		t.Errorf("title = %v, want H1 title", assign["title"])
	}
	cj := assign["content_json"].(map[string]interface{})
	if cj["type"] != "doc" {
		t.Errorf("content_json type = %v, want doc", cj["type"])
	}
}

// A quiz file carries no title. On a module with no assignment the update
// would store an empty title (db-api's Title is a plain string), so the
// import refuses the way import-assignment does (R5).
func TestUpdateModuleContent_QuizWithoutExistingAssignmentRequiresTitle(t *testing.T) {
	data, err := readCompiledModule(writeQuizModuleDir(t, prettyQuiz(t), false))
	if err != nil {
		t.Fatal(err)
	}
	existing := &ExistingModuleData{Status: "DRAFT", Lessons: map[int]map[string]interface{}{}}
	_, err = updateModuleContent(context.Background(), nil, "course-1", data, existing, false, true, false)
	if err == nil || !strings.Contains(err.Error(), "title required") || !strings.Contains(err.Error(), "import-assignment") {
		t.Fatalf("err = %v, want the title-required error pointing at import-assignment", err)
	}
}

// A degraded (206) list that does not show the module must refuse to send:
// db-api may be the missing backend and the module may exist with metadata
// the write would null. With --create it must not create a duplicate either.
func TestImportModule_DegradedListRefusesToSend(t *testing.T) {
	for _, create := range []bool{false, true} {
		stub := &assignmentStub{
			listBodies: []string{chainOnlyListBody(t, "DB API unavailable, showing on-chain data only")},
			listStatus: []int{http.StatusPartialContent},
		}
		c, _ := stub.serve(t)
		_, err := importModule(ImportParams{
			Ctx: context.Background(), Client: c, Config: &config.Config{},
			ModuleDir: writeQuizModuleDir(t, prettyQuiz(t), false), CourseID: "course-1",
			CreateMode: create, DryRun: true, Quiet: true,
		})
		if err == nil || !strings.Contains(err.Error(), "DB API unavailable") || !strings.Contains(err.Error(), "nothing was sent") {
			t.Fatalf("create=%v: err = %v, want a refusal naming the warning", create, err)
		}
		if len(stub.posts) != 0 {
			t.Errorf("create=%v: a degraded list must never lead to a create or update request", create)
		}
	}
}

// The text summary names the quiz digest instead of "Assignment: yes".
func TestCourseImport_TextSummaryReportsQuiz(t *testing.T) {
	bin := buildTestBinary(t)
	stub := &assignmentStub{listBodies: []string{listBody(t, docAssignment("Quiz"), "")}}
	_, url := stub.serve(t)
	dir := writeQuizModuleDir(t, prettyQuiz(t), false)
	stdout, stderr, code := runCLI(t, bin, url, "course", "import", dir, "--course-id", "course-1", "--dry-run")
	if code != 0 {
		t.Fatalf("exit = %d; stderr %q", code, stderr)
	}
	if !strings.Contains(stdout, "Assignment:    quiz (2 questions, threshold 2)") {
		t.Errorf("stdout = %q, want the quiz summary line", stdout)
	}
}

func TestImportResult_AssignmentQuizIsAdditive(t *testing.T) {
	b, _ := json.Marshal(ImportResult{Changes: map[string]interface{}{}})
	if strings.Contains(string(b), "assignment_quiz") {
		t.Error("assignment_quiz must be omitted for a Markdown assignment")
	}
	data, err := readCompiledModule(writeQuizModuleDir(t, prettyQuiz(t), false))
	if err != nil {
		t.Fatal(err)
	}
	b, _ = json.Marshal(ImportResult{AssignmentQuiz: data.AssignmentQuiz, Changes: map[string]interface{}{}})
	if !strings.Contains(string(b), `"assignment_quiz":{"question_count":2,"pass_threshold":2,"question_ids":["q1","q2"]}`) {
		t.Errorf("assignment_quiz not rendered: %s", b)
	}
}
