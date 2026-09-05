package quiz

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

const fixtureRoot = "../../testdata/quiz"

// minimalQuiz mirrors validQuiz in the app's quiz-envelope.test.ts.
const minimalQuiz = `{
  "type": "quiz", "version": 1, "passThreshold": 2,
  "questions": [
    {"id": "q1", "prompt": "What is a wallet?",
     "options": [{"value": "a", "label": "A key manager"}, {"value": "b", "label": "A bank account"}],
     "correctValue": "a"},
    {"id": "q2", "prompt": "What is a credential?",
     "options": [{"value": "a", "label": "A sticker"}, {"value": "b", "label": "An on-chain record"}],
     "correctValue": "b"}
  ]
}`

func codes(issues []Issue) []string {
	out := make([]string, 0, len(issues))
	for _, is := range issues {
		out = append(out, is.Code)
	}
	sort.Strings(out)
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

// readSidecar parses a <case>.issues file: first line "source: app" (both
// apps), "source: app-v2" (andamio-app-v2 only) or "source: cli-additional",
// then one expected issue code per line.
func readSidecar(t *testing.T, path string) (source string, expected []string) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("missing sidecar %s: %v", path, err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	first := true
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if first {
			first = false
			if !strings.HasPrefix(line, "source: ") {
				t.Fatalf("%s: first line must be 'source: <app|app-v2|cli-additional>', got %q", path, line)
			}
			source = strings.TrimPrefix(line, "source: ")
			if source != "app" && source != "app-v2" && source != "cli-additional" {
				t.Fatalf("%s: unknown source %q", path, source)
			}
			continue
		}
		expected = append(expected, line)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if source == "" {
		t.Fatalf("%s: empty sidecar", path)
	}
	if len(expected) == 0 {
		t.Fatalf("%s: sidecar lists no expected codes", path)
	}
	sort.Strings(expected)
	return source, expected
}

func TestValidFixtures(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join(fixtureRoot, "valid", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) < 3 {
		t.Fatalf("expected at least 3 valid fixtures, found %d", len(paths))
	}
	for _, p := range paths {
		name := strings.TrimSuffix(filepath.Base(p), ".json")
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			env, kind, issues, err := Parse(raw)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if kind != Quiz {
				t.Fatalf("kind = %v, want Quiz", kind)
			}
			if len(issues) != 0 {
				t.Fatalf("valid fixture produced issues: %v", issues)
			}
			if name == "minimal" {
				got := Summarize(env)
				want := Summary{QuestionCount: 2, PassThreshold: 2, QuestionIDs: []string{"q1", "q2"}}
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("Summarize = %+v, want %+v", got, want)
				}
			}
		})
	}
}

func TestInvalidFixtures(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join(fixtureRoot, "invalid", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatal("no invalid fixtures found")
	}
	// Every R14 rule must be covered by at least one fixture.
	covered := map[string]bool{}
	for _, p := range paths {
		name := strings.TrimSuffix(filepath.Base(p), ".json")
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			source, expected := readSidecar(t, strings.TrimSuffix(p, ".json")+".issues")
			env, kind, issues, err := Parse(raw)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if kind != Quiz {
				t.Fatalf("kind = %v, want Quiz (invalid fixtures must still be recognized)", kind)
			}
			got := codes(issues)
			if !reflect.DeepEqual(got, expected) {
				t.Fatalf("source=%s issue codes = %v, want %v\nissues: %v", source, got, expected, issues)
			}
			for _, is := range issues {
				if is.Message == "" {
					t.Errorf("issue %s has empty message", is.Code)
				}
				if is.String() == "" {
					t.Errorf("issue %s has empty String()", is.Code)
				}
				covered[is.Code] = true
				isAppV2 := is.Code == CodeEmptyPrompt || is.Code == CodeEmptyOptionLabel || is.Code == CodeEmptyOptionValue
				isAdditional := is.Code == CodeMalformedHelp || is.Code == CodeMalformedIntro
				if source == "app" && (isAppV2 || isAdditional) {
					t.Errorf("source: app fixture yields a code the FCB app does not emit: %s", is.Code)
				}
				if source == "app-v2" && isAdditional {
					t.Errorf("source: app-v2 fixture yields CLI-additional code %s", is.Code)
				}
			}
			// Never let Summarize panic on malformed input.
			_ = Summarize(env)
		})
	}
	for _, code := range AllCodes {
		if code == CodeNotAQuiz {
			continue // guarded directly in TestValidateRefusesNonQuiz; fixtures are all recognized quizzes
		}
		if !covered[code] {
			t.Errorf("no invalid fixture exercises issue code %q", code)
		}
	}
}

func TestRecognize(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want Kind
	}{
		{"doc", `{"type":"doc","content":[]}`, Doc},
		{"quiz", minimalQuiz, Quiz},
		{"quiz unsupported version still recognized", `{"type":"quiz","version":99,"questions":[]}`, Quiz},
		{"quiz evidence is other", `{"type":"quiz-evidence","version":1}`, Other},
		{"quiz without questions is other", `{"type":"quiz","version":1}`, Other},
		{"quiz with string version is other", `{"type":"quiz","version":"1","questions":[]}`, Other},
		{"array", `[{"type":"quiz"}]`, NotObject},
		{"string", `"quiz"`, NotObject},
		{"null", `null`, NotObject},
		{"number", `7`, NotObject},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			kind, env, err := Recognize([]byte(tc.raw))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if kind != tc.want {
				t.Fatalf("kind = %v, want %v", kind, tc.want)
			}
			if tc.want == NotObject && env != nil {
				t.Fatalf("env should be nil for non-objects, got %v", env)
			}
			if tc.want != NotObject && env == nil {
				t.Fatal("env should be non-nil for objects")
			}
		})
	}
}

func TestRecognizeInvalidJSON(t *testing.T) {
	kind, env, err := Recognize([]byte(`{"type": "quiz",`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if kind != NotObject || env != nil {
		t.Fatalf("kind=%v env=%v on invalid JSON", kind, env)
	}
}

func TestIntroNullOrAbsentIsFine(t *testing.T) {
	absent := mustEnv(t, minimalQuiz)
	if issues := Validate(absent); len(issues) != 0 {
		t.Fatalf("absent intro: %v", issues)
	}
	withNull := mustEnv(t, minimalQuiz)
	withNull["intro"] = nil
	if issues := Validate(withNull); len(issues) != 0 {
		t.Fatalf("intro: null: %v", issues)
	}
	// A JSON literal null decodes to a nil interface value under the key.
	raw := strings.Replace(minimalQuiz, `"type": "quiz",`, `"type": "quiz", "intro": null,`, 1)
	_, kind, issues, err := Parse([]byte(raw))
	if err != nil || kind != Quiz {
		t.Fatalf("Parse: kind=%v err=%v", kind, err)
	}
	if len(issues) != 0 {
		t.Fatalf("intro: null via JSON: %v", issues)
	}
}

func TestValidateNamesQuestionIDs(t *testing.T) {
	env := mustEnv(t, minimalQuiz)
	qs := env["questions"].([]interface{})
	q2 := qs[1].(map[string]interface{})
	q2["correctValue"] = "zzz"
	issues := Validate(env)
	if len(issues) != 1 {
		t.Fatalf("want 1 issue, got %v", issues)
	}
	if issues[0].Code != CodeDanglingCorrectValue || issues[0].QuestionID != "q2" {
		t.Fatalf("got %+v", issues[0])
	}
	if !strings.Contains(issues[0].String(), "q2") {
		t.Fatalf("String() should name the question: %q", issues[0].String())
	}
}

func TestValidateRefusesNonQuiz(t *testing.T) {
	// Validate is meant to run after Recognize, but it must not panic or
	// report success when handed something that is not a quiz envelope.
	for _, raw := range []string{
		`{"type":"doc","content":[]}`,
		`{"type":"quiz","version":1}`,
		`{"type":"quiz","version":1,"questions":"nope"}`,
	} {
		issues := Validate(mustEnv(t, raw))
		if len(issues) != 1 || issues[0].Code != CodeNotAQuiz {
			t.Fatalf("%s: got %v, want single %s", raw, issues, CodeNotAQuiz)
		}
	}
	if issues := Validate(nil); len(issues) != 1 || issues[0].Code != CodeNotAQuiz {
		t.Fatalf("nil env: got %v", issues)
	}
}

func TestSummarizeTolerant(t *testing.T) {
	got := Summarize(mustEnv(t, `{"type":"quiz","version":1,"passThreshold":1.5,"questions":[null,{"id":7},{"id":"ok"}]}`))
	want := Summary{QuestionCount: 3, PassThreshold: 0, QuestionIDs: []string{"ok"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Summarize = %+v, want %+v", got, want)
	}
	if got := Summarize(nil); got.QuestionIDs == nil {
		t.Fatal("QuestionIDs must be a non-nil empty slice so JSON emits []")
	}
}
