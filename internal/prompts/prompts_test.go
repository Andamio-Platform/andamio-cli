package prompts

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const fixtureRoot = "../../testdata/prompts"

func codes(issues []Issue) []string {
	out := make([]string, 0, len(issues))
	for _, is := range issues {
		out = append(out, is.Code)
	}
	return out
}

func mustEnv(t *testing.T, raw string) map[string]interface{} {
	t.Helper()
	var env map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		t.Fatalf("fixture JSON: %v", err)
	}
	return env
}

func readFixture(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func fixtures(t *testing.T, dir string) []string {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(fixtureRoot, dir, "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatalf("no fixtures under %s", dir)
	}
	return paths
}

// readSidecar parses a <case>.issues file: first line "source: app", then one
// expected issue code per line, in the order the app emits them.
func readSidecar(t *testing.T, path string) []string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("missing sidecar %s: %v", path, err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	var expected []string
	first := true
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if first {
			first = false
			if line != "source: app" {
				t.Fatalf("%s: first line must be 'source: app', got %q", path, line)
			}
			continue
		}
		expected = append(expected, line)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return expected
}

func TestValidFixtures(t *testing.T) {
	for _, path := range fixtures(t, "valid") {
		t.Run(filepath.Base(path), func(t *testing.T) {
			env, kind, issues, err := Parse(readFixture(t, path))
			if err != nil {
				t.Fatal(err)
			}
			if kind != Definition {
				t.Fatalf("kind = %v, want definition", kind)
			}
			if len(issues) != 0 {
				t.Errorf("issues = %v, want none", issues)
			}
			if env == nil {
				t.Error("env is nil")
			}
		})
	}
}

func TestInvalidFixtures(t *testing.T) {
	for _, path := range fixtures(t, "invalid") {
		t.Run(filepath.Base(path), func(t *testing.T) {
			expected := readSidecar(t, strings.TrimSuffix(path, ".json")+".issues")
			_, kind, issues, err := Parse(readFixture(t, path))
			if err != nil {
				t.Fatal(err)
			}
			if kind != Definition {
				t.Fatalf("kind = %v, want definition (invalid is not unrecognized)", kind)
			}
			if got := codes(issues); !reflect.DeepEqual(got, expected) {
				t.Errorf("codes = %v, want %v", got, expected)
			}
			for _, is := range issues {
				if !contains(AllCodes, is.Code) {
					t.Errorf("code %q is not in AllCodes", is.Code)
				}
			}
		})
	}
}

func TestEvidenceFixtures(t *testing.T) {
	for _, path := range fixtures(t, "evidence/valid") {
		t.Run("valid/"+filepath.Base(path), func(t *testing.T) {
			kind, env, err := Recognize(readFixture(t, path))
			if err != nil {
				t.Fatal(err)
			}
			if kind != Evidence {
				t.Fatalf("kind = %v, want evidence", kind)
			}
			if _, ok := Answers(env); !ok {
				t.Error("Answers reported not-evidence")
			}
		})
	}
	for _, path := range fixtures(t, "evidence/invalid") {
		t.Run("invalid/"+filepath.Base(path), func(t *testing.T) {
			kind, env, err := Recognize(readFixture(t, path))
			if err != nil {
				t.Fatal(err)
			}
			if kind == Evidence {
				t.Fatal("recognized as evidence; the guard is field-strict")
			}
			if _, ok := Answers(env); ok {
				t.Error("Answers accepted non-evidence")
			}
		})
	}
}

func TestRecognize(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want Kind
	}{
		{"tiptap doc", `{"type":"doc","content":[]}`, Other},
		{"quiz", `{"type":"quiz","version":1,"passThreshold":1,"questions":[]}`, Other},
		{"quiz evidence", `{"type":"quiz-evidence","version":1,"answers":[]}`, Other},
		{"prompts without prompts array", `{"type":"prompts","version":1}`, Other},
		{"prompts with string version", `{"type":"prompts","version":"1","prompts":[]}`, Other},
		{"future version still a definition", `{"type":"prompts","version":99,"prompts":[]}`, Definition},
		{"evidence is not a definition", `{"type":"prompts-evidence","version":1,"answers":[]}`, Evidence},
		{"null", `null`, NotObject},
		{"array", `[]`, NotObject},
		{"string", `"prompts"`, NotObject},
		{"number", `3`, NotObject},
		{"bool", `true`, NotObject},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, _, err := Recognize([]byte(tc.raw))
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("kind = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRecognizeInvalidJSON(t *testing.T) {
	if _, _, err := Recognize([]byte(`{`)); err == nil {
		t.Fatal("expected an error for invalid JSON")
	}
}

func TestClassifyNil(t *testing.T) {
	if got := Classify(nil); got != NotObject {
		t.Errorf("Classify(nil) = %v, want not an object", got)
	}
}

// The app checks the intro before returning early on an empty prompts list,
// so both issues are reported, intro first.
func TestValidateCheckOrder(t *testing.T) {
	env := mustEnv(t, `{"type":"prompts","version":2,"intro":"x","prompts":[]}`)
	want := []string{CodeUnsupportedVersion, CodeMalformedIntro, CodeEmptyPrompts}
	if got := codes(Validate(env)); !reflect.DeepEqual(got, want) {
		t.Errorf("codes = %v, want %v", got, want)
	}
}

func TestValidateNamesPromptIDs(t *testing.T) {
	env := mustEnv(t, `{"type":"prompts","version":1,"prompts":[{"id":"cause","label":"The cause"}]}`)
	issues := Validate(env)
	if len(issues) != 1 || issues[0].PromptID != "cause" {
		t.Fatalf("issues = %+v, want one malformed-prompt for cause", issues)
	}
	if !strings.Contains(issues[0].String(), `[prompt "cause"]`) {
		t.Errorf("String() = %q, want the prompt id", issues[0].String())
	}
}

func TestValidateRefusesNonDefinition(t *testing.T) {
	for _, raw := range []string{
		`{"type":"doc","content":[]}`,
		`{"type":"prompts-evidence","version":1,"answers":[]}`,
	} {
		issues := Validate(mustEnv(t, raw))
		if got := codes(issues); !reflect.DeepEqual(got, []string{CodeNotPrompts}) {
			t.Errorf("%s: codes = %v, want [not-prompts]", raw, got)
		}
	}
	if got := codes(Validate(nil)); !reflect.DeepEqual(got, []string{CodeNotPrompts}) {
		t.Errorf("nil: codes = %v, want [not-prompts]", got)
	}
}

func TestSummarize(t *testing.T) {
	env := mustEnv(t, string(readFixture(t, filepath.Join(fixtureRoot, "valid", "minimal.json"))))
	got := Summarize(env)
	want := Summary{PromptCount: 3, PromptIDs: []string{"cause", "value", "ask"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Summarize = %+v, want %+v", got, want)
	}
}

func TestSummarizeTolerant(t *testing.T) {
	for _, env := range []map[string]interface{}{
		nil,
		mustEnv(t, `{"type":"prompts","version":1,"prompts":"nope"}`),
	} {
		s := Summarize(env)
		if s.PromptIDs == nil || s.PromptCount != 0 {
			t.Errorf("Summarize(%v) = %+v, want zero count and non-nil ids", env, s)
		}
	}
	s := Summarize(mustEnv(t, `{"type":"prompts","version":1,"prompts":[null,{"id":3},{"id":"a"}]}`))
	if s.PromptCount != 3 || !reflect.DeepEqual(s.PromptIDs, []string{"a"}) {
		t.Errorf("Summarize skipped wrong entries: %+v", s)
	}
	out, _ := json.Marshal(Summarize(nil))
	if !strings.Contains(string(out), `"prompt_ids":[]`) {
		t.Errorf("JSON = %s, want prompt_ids as []", out)
	}
}

func TestAnswers(t *testing.T) {
	env := mustEnv(t, string(readFixture(t, filepath.Join(fixtureRoot, "evidence", "valid", "minimal.json"))))
	answers, ok := Answers(env)
	if !ok {
		t.Fatal("Answers rejected valid evidence")
	}
	want := []Answer{
		{PromptID: "cause", Label: "The cause", Question: "What should people see?", Answer: "Plastic at matches."},
		{PromptID: "value", Label: "The value", Question: "Which value does it connect to?", Answer: "Teamwork."},
		{PromptID: "ask", Label: "The ask", Question: "What one thing would you ask?", Answer: "Bring a bottle."},
	}
	if !reflect.DeepEqual(answers, want) {
		t.Errorf("answers = %+v, want %+v", answers, want)
	}
	out, _ := json.Marshal(answers[0])
	if string(out) != `{"prompt_id":"cause","label":"The cause","question":"What should people see?","answer":"Plastic at matches."}` {
		t.Errorf("JSON = %s, want snake_case keys", out)
	}
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
