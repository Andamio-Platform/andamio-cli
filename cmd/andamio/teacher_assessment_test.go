package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/Andamio-Platform/andamio-cli/internal/config"
	"github.com/spf13/cobra"
)

// The payload shape is pinned from the schema documented in
// .claude/skills/assess-assignment/SKILL.md, which is the reference the
// existing agent flow was built against:
//
//	{
//	  "alias": "<teacher-alias>",
//	  "course_id": "<course-id>",
//	  "assignment_decisions": [
//	    {"alias": "<student-alias>", "outcome": "accept"},
//	    {"alias": "<student-alias>", "outcome": "refuse"}
//	  ]
//	}
//
// The naming is a trap worth stating plainly: `alias` at the top level is the
// TEACHER, and `alias` inside assignment_decisions is the STUDENT. Two fields,
// same name, two different people. Getting them backwards would assign every
// outcome to the wrong participant and land it on-chain, so this test is
// written from the schema rather than from the implementation.
func TestBuildAssessmentPayload_FieldNaming(t *testing.T) {
	decisions := []AssessmentDecision{
		{Alias: "student-01", Outcome: "accept"},
		{Alias: "student-02", Outcome: "refuse"},
	}

	payload := buildAssessmentPayload("course-abc", "teacher-01", decisions)

	if payload["alias"] != "teacher-01" {
		t.Errorf("top-level alias = %v, want the TEACHER alias %q", payload["alias"], "teacher-01")
	}
	if payload["course_id"] != "course-abc" {
		t.Errorf("course_id = %v, want %q", payload["course_id"], "course-abc")
	}

	raw, ok := payload["assignment_decisions"].([]map[string]string)
	if !ok {
		t.Fatalf("assignment_decisions is %T, want a slice of objects", payload["assignment_decisions"])
	}
	if len(raw) != 2 {
		t.Fatalf("assignment_decisions has %d entries, want 2", len(raw))
	}
	if raw[0]["alias"] != "student-01" {
		t.Errorf("assignment_decisions[0].alias = %q, want the STUDENT alias %q", raw[0]["alias"], "student-01")
	}
	if raw[1]["alias"] != "student-02" {
		t.Errorf("assignment_decisions[1].alias = %q, want %q", raw[1]["alias"], "student-02")
	}
}

// The protocol derives the module from the student's on-chain commitment.
// Sending a module_code would be inventing a field the API does not accept.
func TestBuildAssessmentPayload_HasNoModuleCode(t *testing.T) {
	payload := buildAssessmentPayload("course-abc", "teacher-01", []AssessmentDecision{
		{Alias: "student-01", Outcome: "accept"},
	})

	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, forbidden := range []string{"module_code", "course_module_code", "module"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Errorf("payload contains %q; the protocol derives the module from the on-chain commitment:\n%s", forbidden, encoded)
		}
	}
}

// Decisions must reach the wire in flag order, so the echoed decision set a
// human approves lines up with what they were shown.
func TestBuildAssessmentPayload_PreservesOrder(t *testing.T) {
	decisions := []AssessmentDecision{
		{Alias: "charlie", Outcome: "refuse"},
		{Alias: "alice", Outcome: "accept"},
		{Alias: "bob", Outcome: "accept"},
	}

	payload := buildAssessmentPayload("course-abc", "teacher-01", decisions)
	raw := payload["assignment_decisions"].([]map[string]string)

	for i, want := range []string{"charlie", "alice", "bob"} {
		if raw[i]["alias"] != want {
			t.Errorf("assignment_decisions[%d].alias = %q, want %q", i, raw[i]["alias"], want)
		}
	}
}

func TestParseDecisionFlags_Valid(t *testing.T) {
	got, err := parseDecisionFlags([]string{"student-01=accept", "student-02=refuse"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []AssessmentDecision{
		{Alias: "student-01", Outcome: "accept"},
		{Alias: "student-02", Outcome: "refuse"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d decisions, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("decision %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// Outcome values are lowercase accept/refuse — not pass/fail, not Accept.
// Case is normalized because a shell variable can easily arrive capitalized,
// but an unrecognized *word* is rejected rather than guessed at.
func TestParseDecisionFlags_NormalizesCase(t *testing.T) {
	got, err := parseDecisionFlags([]string{"student-01=ACCEPT", "student-02=Refuse"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got[0].Outcome != "accept" || got[1].Outcome != "refuse" {
		t.Errorf("outcomes = %q, %q; want lowercase accept, refuse", got[0].Outcome, got[1].Outcome)
	}
}

func TestParseDecisionFlags_Rejects(t *testing.T) {
	cases := []struct {
		name    string
		flags   []string
		wantErr string
	}{
		{"unknown outcome", []string{"student-01=pass"}, "accept"},
		{"no separator", []string{"student-01"}, "alias=outcome"},
		{"empty alias", []string{"=accept"}, "alias"},
		{"empty outcome", []string{"student-01="}, "accept"},
		{"no decisions at all", nil, "at least one"},
		{"duplicate alias", []string{"student-01=accept", "student-01=refuse"}, "more than once"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseDecisionFlags(tc.flags)
			if err == nil {
				t.Fatal("expected an error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %q, want it to mention %q", err.Error(), tc.wantErr)
			}
		})
	}
}

// A duplicate alias with two different outcomes must not silently take the
// last one — that would quietly flip a credential decision.
func TestParseDecisionFlags_DuplicateNamesTheAlias(t *testing.T) {
	_, err := parseDecisionFlags([]string{"student-01=accept", "student-02=accept", "student-01=refuse"})
	if err == nil {
		t.Fatal("expected an error for a duplicate alias")
	}
	if !strings.Contains(err.Error(), "student-01") {
		t.Errorf("error does not name the duplicated alias: %q", err.Error())
	}
}

func TestReadDecisionsFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "decisions.json")
	body := `[{"alias":"student-01","outcome":"accept"},{"alias":"student-02","outcome":"refuse"}]`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := readDecisionsFile(path)
	if err != nil {
		t.Fatalf("readDecisionsFile: %v", err)
	}
	if len(got) != 2 || got[0].Alias != "student-01" || got[1].Outcome != "refuse" {
		t.Errorf("decisions = %+v", got)
	}
}

func TestReadDecisionsFile_Rejects(t *testing.T) {
	dir := t.TempDir()

	write := func(name, body string) string {
		t.Helper()
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
		return p
	}

	cases := []struct {
		name    string
		path    string
		wantErr string
	}{
		{"missing file", filepath.Join(dir, "nope.json"), "read"},
		{"invalid json", write("bad.json", "{not json"), "parse"},
		{"empty array", write("empty.json", "[]"), "at least one"},
		{"bad outcome", write("badoutcome.json", `[{"alias":"a","outcome":"pass"}]`), "accept"},
		{"missing alias", write("noalias.json", `[{"outcome":"accept"}]`), "alias"},
		{"duplicate alias", write("dupe.json", `[{"alias":"a","outcome":"accept"},{"alias":"a","outcome":"refuse"}]`), "more than once"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := readDecisionsFile(tc.path); err == nil {
				t.Fatal("expected an error, got nil")
			} else if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %q, want it to mention %q", err.Error(), tc.wantErr)
			}
		})
	}
}

// --- wire behavior ---------------------------------------------------------

// captureAssessRequest stands up a stub assess endpoint and returns the
// decoded request body along with every path the client touched.
func captureAssessRequest(t *testing.T, respBody string) (*client.Client, *[]string, *map[string]interface{}) {
	t.Helper()

	paths := []string{}
	body := map[string]interface{}{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(respBody))
	}))
	t.Cleanup(srv.Close)

	return client.New(&config.Config{BaseURL: srv.URL}), &paths, &body
}

// The request that reaches the wire must carry the schema's field naming.
// Asserting on the decoded HTTP body — not on buildAssessmentPayload's return
// value — closes the gap where the payload builder is correct but the command
// sends something else.
func TestAssessmentBuild_WireRequestShape(t *testing.T) {
	c, paths, body := captureAssessRequest(t, `{"unsigned_tx":"84a400d9"}`)

	decisions := []AssessmentDecision{
		{Alias: "student-01", Outcome: "accept"},
		{Alias: "student-02", Outcome: "refuse"},
	}
	payload := buildAssessmentPayload("course-abc", "teacher-01", decisions)

	var resp map[string]interface{}
	if err := c.Post(t.Context(), assessEndpoint, payload, &resp); err != nil {
		t.Fatalf("POST: %v", err)
	}

	if got := (*body)["alias"]; got != "teacher-01" {
		t.Errorf("wire alias = %v, want the TEACHER alias", got)
	}
	if got := (*body)["course_id"]; got != "course-abc" {
		t.Errorf("wire course_id = %v", got)
	}

	raw, ok := (*body)["assignment_decisions"].([]interface{})
	if !ok {
		t.Fatalf("wire assignment_decisions is %T", (*body)["assignment_decisions"])
	}
	if len(raw) != 2 {
		t.Fatalf("wire carried %d decisions, want 2", len(raw))
	}
	first := raw[0].(map[string]interface{})
	if first["alias"] != "student-01" || first["outcome"] != "accept" {
		t.Errorf("wire decision[0] = %v", first)
	}

	// Exactly one request, to the assess endpoint. Building must not sign or submit.
	if len(*paths) != 1 {
		t.Errorf("made %d requests, want exactly 1 (build only): %v", len(*paths), *paths)
	}
	if (*paths)[0] != assessEndpoint {
		t.Errorf("called %q, want %q", (*paths)[0], assessEndpoint)
	}
	for _, p := range *paths {
		if strings.Contains(p, "submit") || strings.Contains(p, "register") {
			t.Errorf("build reached a submit/register path: %q", p)
		}
	}
}

// --- rendering -------------------------------------------------------------

func sampleEnvelope() AssessmentBuildEnvelope {
	decisions := []AssessmentDecision{
		{Alias: "student-01", Outcome: "accept"},
		{Alias: "student-02", Outcome: "refuse"},
		{Alias: "student-03", Outcome: "refuse"},
	}
	return AssessmentBuildEnvelope{
		UnsignedTx:    "84a400d9010281825820",
		CourseID:      "course-abc",
		TeacherAlias:  "teacher-01",
		Decisions:     decisions,
		DecisionCount: len(decisions),
	}
}

// A reviewer must be able to see every decision, and the tally must match.
func TestRenderAssessmentBuild_ShowsEveryDecision(t *testing.T) {
	var buf strings.Builder
	renderAssessmentBuild(&buf, sampleEnvelope())
	got := buf.String()

	for _, want := range []string{
		"student-01", "student-02", "student-03",
		"ACCEPT", "REFUSE",
		"course-abc", "teacher-01",
		"3 decision(s): 1 accept, 2 refuse",
		"84a400d9010281825820",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered output missing %q:\n%s", want, got)
		}
	}
}

// Decisions before the CBOR: a reviewer should meet what they are approving
// before they meet the blob they cannot read.
func TestRenderAssessmentBuild_DecisionsPrecedeTransaction(t *testing.T) {
	var buf strings.Builder
	renderAssessmentBuild(&buf, sampleEnvelope())
	got := buf.String()

	if strings.Index(got, "student-01") > strings.Index(got, "Unsigned TX") {
		t.Errorf("transaction is rendered before the decisions:\n%s", got)
	}
}

func TestAssessmentBuildEnvelope_JSONShape(t *testing.T) {
	encoded, err := json.Marshal(sampleEnvelope())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(encoded, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	for _, key := range []string{"unsigned_tx", "course_id", "teacher_alias", "decisions", "decision_count"} {
		if _, ok := parsed[key]; !ok {
			t.Errorf("envelope is missing %q: %s", key, encoded)
		}
	}
	if parsed["decision_count"] != float64(3) {
		t.Errorf("decision_count = %v, want 3", parsed["decision_count"])
	}
	decisions, ok := parsed["decisions"].([]interface{})
	if !ok || len(decisions) != 3 {
		t.Fatalf("decisions = %v", parsed["decisions"])
	}
	if len(decisions) != int(parsed["decision_count"].(float64)) {
		t.Error("decision_count disagrees with len(decisions)")
	}
	first := decisions[0].(map[string]interface{})
	if first["alias"] != "student-01" || first["outcome"] != "accept" {
		t.Errorf("decisions[0] = %v", first)
	}
}

// --- command wiring --------------------------------------------------------

// R11: the single-step path must survive. Building separately is an addition,
// not a replacement.
func TestTxRunStillRegistered(t *testing.T) {
	if cmd, ok := resolve("tx run"); !ok || cmd.Hidden {
		t.Error("tx run is missing or hidden; the single-step path must still work")
	}
	for _, path := range []string{"tx build", "tx sign", "tx submit", "tx register"} {
		if cmd, ok := resolve(path); !ok || cmd.Hidden {
			t.Errorf("%q is missing or hidden", path)
		}
	}
}

func TestTeacherAssessmentBuild_IsRegisteredAndGated(t *testing.T) {
	cmd, ok := resolve("teacher assessment build")
	if !ok {
		t.Fatal("teacher assessment build is not registered")
	}
	if cmd.Hidden {
		t.Error("teacher assessment build is hidden")
	}
	for _, flag := range []string{"course-id", "alias", "decision", "decisions-file"} {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("missing --%s flag", flag)
		}
	}
}

// Building an assessment must require auth before anything reaches the network.
func TestTeacherAssessmentBuild_RequiresAuth(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".andamio")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Config with no user JWT.
	b, _ := json.Marshal(map[string]string{"base_url": "http://127.0.0.1:1", "api_key": "k"})
	if err := os.WriteFile(filepath.Join(dir, "config.json"), b, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cmd, _ := resolve("teacher assessment build")

	// Cobra resolves PersistentPreRunE by walking up from the leaf and running
	// the first one it finds. Walking the same way here asserts the property
	// that matters — auth is enforced somewhere in this command's chain —
	// rather than pinning which level declares it.
	var gate func(*cobra.Command, []string) error
	for c := cmd; c != nil; c = c.Parent() {
		if c.PersistentPreRunE != nil {
			gate = c.PersistentPreRunE
			break
		}
	}
	if gate == nil {
		t.Fatal("no PersistentPreRunE anywhere in the chain; building an assessment is ungated")
	}

	err := gate(cmd, nil)
	if err == nil {
		t.Fatal("expected an auth error with no user JWT configured")
	}
	if !strings.Contains(err.Error(), "not authenticated") {
		t.Errorf("error = %q, want an authentication error", err.Error())
	}
}
