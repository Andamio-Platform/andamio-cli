package main

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// Titles reports each lesson, introduction and assignment title against the
// file it was written to. Untitled files and outline.md have no entry.
func TestWriteCompiledModule_Titles(t *testing.T) {
	data := exportModuleData(&exportContent{ContentJSON: quizEnvelope(), Title: "Module Quiz"})
	data.SLTs = []SLTData{
		{Index: 1, Text: "One", Lesson: &exportContent{Title: "What is\nCardano? ", ContentJSON: lessonDoc("Hi.")}},
		{Index: 2, Text: "Two", Lesson: &exportContent{ContentJSON: lessonDoc("No title.")}},
	}
	data.Introduction = &exportContent{Title: "Welcome", ContentJSON: lessonDoc("Hello.")}

	result, err := writeCompiledModule(t.TempDir(), data)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"lesson-1.md":          "What is Cardano?", // cleaned like the H1
		"introduction.md":      "Welcome",
		"assignment.quiz.json": "Module Quiz", // a quiz has no H1, so this is the only place to find it
	}
	if !reflect.DeepEqual(result.Titles, want) {
		t.Errorf("Titles = %v, want %v", result.Titles, want)
	}
}

// With no titles at all, the field is left out of the JSON.
func TestExportResult_NoTitlesOmitted(t *testing.T) {
	data := exportModuleData(nil)
	data.SLTs[0].Lesson = &exportContent{ContentJSON: lessonDoc("Hi.")}

	result, err := writeCompiledModule(t.TempDir(), data)
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(ExportResult{Files: result.Files, Titles: result.Titles})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "titles") {
		t.Errorf("JSON = %s, want no titles field", b)
	}
}
