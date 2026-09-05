package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// quizEnvelope is a valid v1 quiz used across the export/import round-trip
// tests. Numbers are float64 because that is what encoding/json decodes into,
// and the comparisons below are structural, never byte-for-byte (KTD3).
func quizEnvelope() map[string]interface{} {
	return map[string]interface{}{
		"type":          "quiz",
		"version":       float64(1),
		"passThreshold": float64(2),
		"questions": []interface{}{
			map[string]interface{}{
				"id":     "q1",
				"prompt": "What is a wallet?",
				"options": []interface{}{
					map[string]interface{}{"value": "a", "label": "A key manager"},
					map[string]interface{}{"value": "b", "label": "A bank account"},
				},
				"correctValue": "a",
			},
			map[string]interface{}{
				"id":     "q2",
				"prompt": "What is a credential?",
				"options": []interface{}{
					map[string]interface{}{"value": "a", "label": "A sticker"},
					map[string]interface{}{"value": "b", "label": "An on-chain record"},
				},
				"correctValue": "b",
			},
		},
	}
}

// wrapAssignment mirrors the shape fetchModuleData hands writeCompiledModule.
func wrapAssignment(contentJSON map[string]interface{}, title string) map[string]interface{} {
	return map[string]interface{}{
		"data": map[string]interface{}{
			"content": map[string]interface{}{
				"content_json": contentJSON,
				"title":        title,
			},
		},
	}
}

func exportModuleData(assignment map[string]interface{}) *ModuleData {
	return &ModuleData{
		CourseID:   "course-1",
		CourseSlug: "course",
		ModuleCode: "101",
		Title:      "Module 101",
		Status:     "DRAFT",
		SLTs:       []SLTData{{Index: 1, Text: "Do a thing"}},
		Assignment: assignment,
	}
}

func fileExists(t *testing.T, path string) bool {
	t.Helper()
	_, err := os.Stat(path)
	if err == nil {
		return true
	}
	if os.IsNotExist(err) {
		return false
	}
	t.Fatalf("stat %s: %v", path, err)
	return false
}

// A non-doc assignment is preserved verbatim on disk as assignment.quiz.json;
// converting it to Markdown matched no node type and produced an empty
// assignment.md that a later import would publish as the assignment (#165).
func TestWriteCompiledModule_QuizAssignmentWritesQuizJSON(t *testing.T) {
	dir := t.TempDir()
	quiz := quizEnvelope()

	result, err := writeCompiledModule(dir, exportModuleData(wrapAssignment(quiz, "Module Quiz")))
	if err != nil {
		t.Fatalf("writeCompiledModule: %v", err)
	}

	quizPath := filepath.Join(dir, "assignment.quiz.json")
	if !fileExists(t, quizPath) {
		t.Fatal("assignment.quiz.json was not written")
	}
	if fileExists(t, filepath.Join(dir, "assignment.md")) {
		t.Error("assignment.md must not be written for a quiz assignment")
	}
	if !slices.Contains(result.Files, "assignment.quiz.json") {
		t.Errorf("Files = %v, want assignment.quiz.json listed", result.Files)
	}
	if slices.Contains(result.Files, "assignment.md") {
		t.Errorf("Files = %v, must not list assignment.md", result.Files)
	}

	raw, err := os.ReadFile(quizPath)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("assignment.quiz.json is not JSON: %v", err)
	}
	if !reflect.DeepEqual(got, quiz) {
		t.Errorf("assignment.quiz.json content differs from the stored envelope\n got: %v\nwant: %v", got, quiz)
	}
}

func TestWriteCompiledModule_DocAssignmentUnchanged(t *testing.T) {
	dir := t.TempDir()
	doc := map[string]interface{}{
		"type": "doc",
		"content": []interface{}{
			map[string]interface{}{
				"type":    "paragraph",
				"content": []interface{}{map[string]interface{}{"type": "text", "text": "Write an essay."}},
			},
		},
	}

	result, err := writeCompiledModule(dir, exportModuleData(wrapAssignment(doc, "Essay")))
	if err != nil {
		t.Fatalf("writeCompiledModule: %v", err)
	}
	if fileExists(t, filepath.Join(dir, "assignment.quiz.json")) {
		t.Error("assignment.quiz.json must not be written for a doc assignment")
	}
	md, err := os.ReadFile(filepath.Join(dir, "assignment.md"))
	if err != nil {
		t.Fatalf("assignment.md missing: %v", err)
	}
	if want := "# Essay\n\nWrite an essay."; strings.TrimSpace(string(md)) != want {
		t.Errorf("assignment.md = %q, want H1 title then body", string(md))
	}
	if !slices.Contains(result.Files, "assignment.md") {
		t.Errorf("Files = %v, want assignment.md listed", result.Files)
	}
}

func TestWriteCompiledModule_NoAssignmentWritesNeither(t *testing.T) {
	dir := t.TempDir()
	if _, err := writeCompiledModule(dir, exportModuleData(nil)); err != nil {
		t.Fatalf("writeCompiledModule: %v", err)
	}
	if fileExists(t, filepath.Join(dir, "assignment.md")) || fileExists(t, filepath.Join(dir, "assignment.quiz.json")) {
		t.Error("no assignment file expected when the module has no assignment")
	}
}

// When the remote assignment is gone, both stale assignment files go too —
// otherwise the next import would republish the deleted assignment.
func TestWriteCompiledModule_NoAssignmentRemovesStaleFiles(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"assignment.md", "assignment.quiz.json"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("stale"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := writeCompiledModule(dir, exportModuleData(nil)); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"assignment.md", "assignment.quiz.json"} {
		if fileExists(t, filepath.Join(dir, name)) {
			t.Errorf("stale %s survived an export of a module with no assignment", name)
		}
	}
}

// A type-less object is not a Tiptap doc: exporting it as Markdown would be
// lossy (an H1-only file), so it takes the verbatim path like any non-doc.
func TestWriteCompiledModule_TypelessObjectExportsVerbatim(t *testing.T) {
	dir := t.TempDir()
	if _, err := writeCompiledModule(dir, exportModuleData(wrapAssignment(map[string]interface{}{"foo": "bar"}, "Odd"))); err != nil {
		t.Fatal(err)
	}
	if !fileExists(t, filepath.Join(dir, "assignment.quiz.json")) || fileExists(t, filepath.Join(dir, "assignment.md")) {
		t.Error("a type-less content_json must be preserved verbatim, not flattened to Markdown")
	}
}

// A re-export into the same directory (only reachable under --force) must not
// leave the previous assignment file behind: both files present is the R11
// ambiguity error on the next import, and export would be the tool that
// produced it.
func TestWriteCompiledModule_ReexportRemovesStaleCounterpart(t *testing.T) {
	dir := t.TempDir()
	stale := filepath.Join(dir, "assignment.md")
	if err := os.WriteFile(stale, []byte("# Old\n\nstale"), 0644); err != nil {
		t.Fatal(err)
	}

	if _, err := writeCompiledModule(dir, exportModuleData(wrapAssignment(quizEnvelope(), "Quiz"))); err != nil {
		t.Fatalf("writeCompiledModule: %v", err)
	}
	if fileExists(t, stale) {
		t.Error("stale assignment.md survived a quiz export")
	}
	if !fileExists(t, filepath.Join(dir, "assignment.quiz.json")) {
		t.Fatal("assignment.quiz.json was not written")
	}

	// Reverse transition: quiz back to doc removes the stale quiz file.
	doc := map[string]interface{}{"type": "doc", "content": []interface{}{}}
	if _, err := writeCompiledModule(dir, exportModuleData(wrapAssignment(doc, "Essay"))); err != nil {
		t.Fatalf("writeCompiledModule: %v", err)
	}
	if fileExists(t, filepath.Join(dir, "assignment.quiz.json")) {
		t.Error("stale assignment.quiz.json survived a doc export")
	}
	if !fileExists(t, stale) {
		t.Error("assignment.md was not written on the reverse transition")
	}
}
