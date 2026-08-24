package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// runCLI invokes the built binary with HOME pointed at a scratch config so the
// developer's real credentials are never used, and returns stdout, stderr and
// the exit code.
func runCLI(t *testing.T, bin, baseURL string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	// The undecodable "test-jwt" passes auth gating (fail-open contract) so
	// the request actually reaches the stub; the stub decides what status
	// comes back.
	return runCLIWithJWT(t, bin, baseURL, "test-jwt", nil, args...)
}

func statusStub(t *testing.T, status int) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"message":"stub"}`))
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// The contract issue #126 turns on: each distinguishable failure gets its own
// exit code AND its own kind, and the two never disagree.
func TestExitCodes_AndKindsAgree(t *testing.T) {
	bin := buildTestBinary(t)

	cases := []struct {
		name     string
		status   int
		wantCode int
		wantKind string
	}{
		{"not found", http.StatusNotFound, 2, "not_found"},
		{"unauthorized", http.StatusUnauthorized, 3, "auth"},
		{"forbidden", http.StatusForbidden, 3, "auth"},
		{"conflict", http.StatusConflict, 6, "conflict"},
		{"server error", http.StatusInternalServerError, 1, "server"},
		{"backpressure", http.StatusTooManyRequests, 1, "backpressure"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			url := statusStub(t, tc.status)
			stdout, _, code := runCLI(t, bin, url, "course", "list", "--output", "json")

			if code != tc.wantCode {
				t.Errorf("exit code = %d, want %d", code, tc.wantCode)
			}

			var parsed map[string]string
			if err := json.Unmarshal([]byte(stdout), &parsed); err != nil {
				t.Fatalf("stdout is not JSON: %v\nraw: %q", err, stdout)
			}
			if parsed["kind"] != tc.wantKind {
				t.Errorf("kind = %q, want %q", parsed["kind"], tc.wantKind)
			}
			if parsed["error"] == "" {
				t.Error("error message is empty")
			}
		})
	}
}

// "Could not reach the service" gets its own code. Before 1.0 this was exit 1,
// indistinguishable from a malformed flag or a decode failure.
func TestExitCodes_UnreachableServiceIsFive(t *testing.T) {
	bin := buildTestBinary(t)

	// Reserve a port, then release it, so nothing is listening.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close()

	stdout, _, code := runCLI(t, bin, url, "course", "list", "--output", "json")

	if code != 5 {
		t.Errorf("exit code = %d, want 5 (unreachable)", code)
	}
	var parsed map[string]string
	if err := json.Unmarshal([]byte(stdout), &parsed); err != nil {
		t.Fatalf("stdout is not JSON: %v\nraw: %q", err, stdout)
	}
	if parsed["kind"] != "unreachable" {
		t.Errorf("kind = %q, want %q", parsed["kind"], "unreachable")
	}
}

func TestExitCodes_RemovedCommandIsFour(t *testing.T) {
	bin := buildTestBinary(t)
	url := statusStub(t, http.StatusOK)

	stdout, _, code := runCLI(t, bin, url, "course", "student", "claim", "--output", "json")

	if code != 4 {
		t.Errorf("exit code = %d, want 4 (removed command)", code)
	}
	var parsed map[string]string
	if err := json.Unmarshal([]byte(stdout), &parsed); err != nil {
		t.Fatalf("stdout is not JSON: %v\nraw: %q", err, stdout)
	}
	if parsed["kind"] != "removed_command" {
		t.Errorf("kind = %q, want %q", parsed["kind"], "removed_command")
	}
}

// The three outcomes issue #126 names by name must be mutually distinguishable
// on the same command. This is the assertion that would have failed before 1.0:
// "nothing found" and "could not reach the service" both looked like success or
// exit 1 depending on the path.
func TestExitCodes_ThreeOutcomesAreDistinct(t *testing.T) {
	bin := buildTestBinary(t)

	// Nothing found: a valid empty result set.
	emptySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer emptySrv.Close()

	// Not permitted.
	forbiddenURL := statusStub(t, http.StatusForbidden)

	// Unreachable.
	deadSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	deadURL := deadSrv.URL
	deadSrv.Close()

	emptyOut, _, emptyCode := runCLI(t, bin, emptySrv.URL, "course", "list", "--output", "json")
	_, _, forbiddenCode := runCLI(t, bin, forbiddenURL, "course", "list", "--output", "json")
	_, _, deadCode := runCLI(t, bin, deadURL, "course", "list", "--output", "json")

	if emptyCode != 0 {
		t.Errorf("empty result exited %d, want 0 — nothing found is not an error", emptyCode)
	}
	if !strings.Contains(emptyOut, `"data"`) {
		t.Errorf("empty result did not emit a data collection: %q", emptyOut)
	}
	if forbiddenCode != 3 {
		t.Errorf("forbidden exited %d, want 3", forbiddenCode)
	}
	if deadCode != 5 {
		t.Errorf("unreachable exited %d, want 5", deadCode)
	}

	codes := map[int]string{emptyCode: "empty", forbiddenCode: "forbidden", deadCode: "unreachable"}
	if len(codes) != 3 {
		t.Errorf("the three outcomes are not distinguishable by exit code: %v", codes)
	}
}

// Text mode must be unchanged: kind is a machine-readable field, and leaking it
// into human output would be a regression in the other direction.
func TestExitCodes_TextModeCarriesNoKind(t *testing.T) {
	bin := buildTestBinary(t)
	url := statusStub(t, http.StatusNotFound)

	stdout, stderr, code := runCLI(t, bin, url, "course", "list")

	if code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if strings.Contains(stderr, "kind") || strings.Contains(stdout, "kind") {
		t.Errorf("text mode leaked the kind field:\nstdout: %q\nstderr: %q", stdout, stderr)
	}
	if stdout != "" {
		t.Errorf("error output went to stdout: %q", stdout)
	}
	if stderr == "" {
		t.Error("no error message on stderr")
	}
}

// --- agent-driven path audit (U7) ------------------------------------------

// courseWithCommitments stubs the assignment-commitments endpoint.
func commitmentsStub(t *testing.T, body string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// Both not-found branches of 'teacher assignments get' must classify the same
// way. Before 1.0 the malformed/missing-data branch returned an untyped error
// and exited 1 — indistinguishable from a server failure — while the
// no-matching-student branch three lines later exited 2.
func TestAgentPaths_AssignmentsGet_BothNotFoundBranchesExitTwo(t *testing.T) {
	bin := buildTestBinary(t)

	cases := []struct {
		name string
		body string
	}{
		{"data field absent", `{}`},
		{"data field is not an array", `{"data":"unexpected"}`},
		{"data field is null", `{"data":null}`},
		{"no matching student", `{"data":[{"course_module_code":"101","student_alias":"someone-else"}]}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			url := commitmentsStub(t, tc.body)
			stdout, _, code := runCLI(t, bin, url,
				"teacher", "assignments", "get", "course-1", "101", "student-01", "--output", "json")

			if code != 2 {
				t.Errorf("exit code = %d, want 2 (not found)", code)
			}
			var parsed map[string]string
			if err := json.Unmarshal([]byte(stdout), &parsed); err != nil {
				t.Fatalf("stdout is not JSON: %v\nraw: %q", err, stdout)
			}
			if parsed["kind"] != "not_found" {
				t.Errorf("kind = %q, want %q", parsed["kind"], "not_found")
			}
		})
	}
}

// The outcome those not-found branches must stay distinct from.
func TestAgentPaths_AssignmentsGet_ServerFailureIsNotNotFound(t *testing.T) {
	bin := buildTestBinary(t)
	url := statusStub(t, http.StatusInternalServerError)

	stdout, _, code := runCLI(t, bin, url,
		"teacher", "assignments", "get", "course-1", "101", "student-01", "--output", "json")

	if code == 2 {
		t.Error("a server failure exited 2; it must not look like not-found")
	}
	var parsed map[string]string
	_ = json.Unmarshal([]byte(stdout), &parsed)
	if parsed["kind"] != "server" {
		t.Errorf("kind = %q, want %q", parsed["kind"], "server")
	}
}

// The three-way distinction, on the assessment paths an agent actually drives.
func TestAgentPaths_AssessmentReads_ThreeOutcomesDistinct(t *testing.T) {
	bin := buildTestBinary(t)

	empty := commitmentsStub(t, `{"data":[]}`)
	forbidden := statusStub(t, http.StatusForbidden)

	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	deadURL := dead.URL
	dead.Close()

	commands := [][]string{
		{"teacher", "assignments", "list", "--course", "course-1", "--output", "json"},
		{"project", "manager", "commitments", "--project-id", "proj-1", "--output", "json"},
	}

	for _, args := range commands {
		t.Run(strings.Join(args[:3], " "), func(t *testing.T) {
			emptyOut, _, emptyCode := runCLI(t, bin, empty, args...)
			_, _, forbiddenCode := runCLI(t, bin, forbidden, args...)
			_, _, deadCode := runCLI(t, bin, deadURL, args...)

			if emptyCode != 0 {
				t.Errorf("empty result exited %d, want 0", emptyCode)
			}
			if !strings.Contains(emptyOut, "data") {
				t.Errorf("empty result did not emit a data collection: %q", emptyOut)
			}
			if forbiddenCode != 3 {
				t.Errorf("forbidden exited %d, want 3", forbiddenCode)
			}
			if deadCode != 5 {
				t.Errorf("unreachable exited %d, want 5", deadCode)
			}
		})
	}
}

// The empty-set contract, pinned on the shared list helper so a future
// "improvement" that turns empty into an error has to break a test first.
func TestAgentPaths_EmptyListIsSuccessNotError(t *testing.T) {
	bin := buildTestBinary(t)
	url := commitmentsStub(t, `{"data":[]}`)

	t.Run("json emits an empty collection on stdout", func(t *testing.T) {
		stdout, _, code := runCLI(t, bin, url, "course", "list", "--output", "json")
		if code != 0 {
			t.Errorf("exit code = %d, want 0", code)
		}
		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(stdout), &parsed); err != nil {
			t.Fatalf("stdout is not JSON: %v\nraw: %q", err, stdout)
		}
		data, ok := parsed["data"].([]interface{})
		if !ok {
			t.Fatalf("data is %T, want an array", parsed["data"])
		}
		if len(data) != 0 {
			t.Errorf("data has %d entries, want 0", len(data))
		}
		if _, hasErr := parsed["error"]; hasErr {
			t.Error("empty result emitted an error field")
		}
	})

	t.Run("text puts the notice on stderr and nothing on stdout", func(t *testing.T) {
		stdout, stderr, code := runCLI(t, bin, url, "course", "list")
		if code != 0 {
			t.Errorf("exit code = %d, want 0", code)
		}
		if strings.TrimSpace(stdout) != "" {
			t.Errorf("stdout carried the empty notice: %q", stdout)
		}
		if !strings.Contains(strings.ToLower(stderr), "no ") {
			t.Errorf("stderr has no empty-result notice: %q", stderr)
		}
	})
}

// A bare top-level JSON array is a real gateway shape, not a hypothetical:
// /api/v2/tx/pending returns `[]` when nothing is pending, and the 2.5 gateway
// serves several list routes unwrapped. getJSON used to decode into
// map[string]interface{}, which turned those responses into a hard unmarshal
// failure — exit 1, kind "error", and a Go-internals message on stdout — in
// direct violation of the empty-set contract pinned above. The output layer
// already handled top-level arrays; only the decode target did not.
func TestGetJSON_BareArrayResponseIsSuccessNotDecodeError(t *testing.T) {
	bin := buildTestBinary(t)

	for _, tc := range []struct {
		name string
		body string
		want int
	}{
		{"empty array", `[]`, 0},
		{"populated array", `[{"tx_hash":"abc","tx_type":"teachers_update"}]`, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.body))
			}))
			t.Cleanup(srv.Close)

			stdout, _, code := runCLI(t, bin, srv.URL, "tx", "pending", "--output", "json")
			if code != 0 {
				t.Fatalf("exit code = %d, want 0 — a bare array is a valid response, not a failure.\nstdout: %q", code, stdout)
			}
			var parsed []interface{}
			if err := json.Unmarshal([]byte(stdout), &parsed); err != nil {
				t.Fatalf("stdout is not a JSON array: %v\nraw: %q", err, stdout)
			}
			if len(parsed) != tc.want {
				t.Errorf("got %d entries, want %d", len(parsed), tc.want)
			}
			if strings.Contains(stdout, "cannot unmarshal") {
				t.Errorf("Go-internals decode error leaked to stdout: %q", stdout)
			}
		})
	}
}
