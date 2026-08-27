package main

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
)

const fixtureV25PartialContent = "../../internal/client/testdata/v2-5-partial-content-response.json"

func TestMetaWarning_Extracts(t *testing.T) {
	cases := []struct {
		name string
		in   interface{}
		want string
	}{
		{"nested warning", map[string]interface{}{"data": 1, "meta": map[string]interface{}{"warning": " degraded "}}, "degraded"},
		{"no meta", map[string]interface{}{"data": 1}, ""},
		{"meta without warning", map[string]interface{}{"meta": map[string]interface{}{"page": 2}}, ""},
		{"non-string warning", map[string]interface{}{"meta": map[string]interface{}{"warning": 42}}, ""},
		{"bare array", []interface{}{1, 2}, ""},
		{"nil", nil, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := metaWarning(tc.in); got != tc.want {
				t.Errorf("metaWarning = %q, want %q", got, tc.want)
			}
		})
	}
}

// A 206 with data is a success (exit 0), like an empty result is: the data is
// there, degraded. JSON mode keeps meta.warning in the envelope on stdout;
// text mode prints it on stderr and the data on stdout (#157).
func TestExitCodes_206PartialContentIsSuccess(t *testing.T) {
	body, err := os.ReadFile(fixtureV25PartialContent)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	bin := buildTestBinary(t)
	url := statusStubWithBody(t, http.StatusPartialContent, string(body))

	t.Run("json", func(t *testing.T) {
		stdout, _, code := runCLI(t, bin, url, "project", "get", "0000", "--output", "json")
		if code != 0 {
			t.Fatalf("exit code = %d, want 0 — a partial result is not an error\nstdout: %s", code, stdout)
		}
		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(stdout), &parsed); err != nil {
			t.Fatalf("stdout is not JSON: %v\nraw: %q", err, stdout)
		}
		if _, ok := parsed["error"]; ok {
			t.Errorf("206 rendered as an error envelope: %s", stdout)
		}
		if metaWarning(parsed) == "" {
			t.Errorf("meta.warning dropped from the JSON envelope: %s", stdout)
		}
	})

	t.Run("text", func(t *testing.T) {
		stdout, stderr, code := runCLI(t, bin, url, "project", "get", "0000")
		if code != 0 {
			t.Fatalf("exit code = %d, want 0\nstderr: %s", code, stderr)
		}
		if !strings.Contains(stderr, "warning: DB API unavailable") {
			t.Errorf("text mode should surface meta.warning on stderr, got %q", stderr)
		}
		if !strings.Contains(stdout, "chain_only") {
			t.Errorf("text mode should still print the data on stdout, got %q", stdout)
		}
	})
}

// List commands unwrap `data` and emit a bare array in JSON mode; a degraded
// (206) list must keep that exact shape — a script's `jq '.[]'` cannot start
// failing because a backend was down — and say so on stderr in every mode.
// The empty-list envelope `{"data":[]}` is already an object, so it keeps meta.
func TestExitCodes_206OnList_ShapeStableWarningOnStderr(t *testing.T) {
	bin := buildTestBinary(t)
	const warning = "DB API unavailable, showing on-chain data only"
	body := `{"data":[{"course_id":"c1","title":"One"}],"meta":{"warning":"` + warning + `"}}`
	url := statusStubWithBody(t, http.StatusPartialContent, body)

	stdout, stderr, code := runCLI(t, bin, url, "course", "list", "--output", "json")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0\nstdout: %s", code, stdout)
	}
	var arr []map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &arr); err != nil || len(arr) != 1 || arr[0]["course_id"] != "c1" {
		t.Fatalf("degraded list must keep the bare-array shape (err=%v): %s", err, stdout)
	}
	if !strings.Contains(stderr, "warning: "+warning) {
		t.Errorf("JSON mode should still surface the degradation on stderr, got %q", stderr)
	}

	_, stderr, code = runCLI(t, bin, url, "course", "list")
	if code != 0 || !strings.Contains(stderr, "warning: "+warning) {
		t.Errorf("text mode: code=%d stderr=%q", code, stderr)
	}

	// Empty degraded list: the {"data":[]} envelope keeps meta.
	url = statusStubWithBody(t, http.StatusPartialContent, `{"data":[],"meta":{"warning":"`+warning+`"}}`)
	stdout, _, code = runCLI(t, bin, url, "course", "list", "--output", "json")
	var parsed map[string]interface{}
	if code != 0 || json.Unmarshal([]byte(stdout), &parsed) != nil || metaWarning(parsed) != warning {
		t.Errorf("empty degraded list: code=%d stdout=%s", code, stdout)
	}
}

// One positional and no --course: the missing argument is the module code.
// The old message said "course-id required" — blaming the one argument that
// was supplied (#157).
func TestCourseSlts_OneArgNoCourseFlag_NamesModuleCode(t *testing.T) {
	bin := buildTestBinary(t)
	url := statusStubWithBody(t, http.StatusOK, `{"data":[]}`)

	_, stderr, code := runCLI(t, bin, url, "course", "slts", "some-course-id")
	if code == 0 {
		t.Fatal("expected non-zero exit")
	}
	if !strings.Contains(stderr, "module-code required") {
		t.Errorf("stderr = %q, want 'module-code required'", stderr)
	}
	if strings.Contains(stderr, "course-id required") {
		t.Errorf("stderr still blames the course id: %q", stderr)
	}
}
