package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/Andamio-Platform/andamio-cli/internal/apierr"
	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/Andamio-Platform/andamio-cli/internal/config"
)

// assignmentStub is a gateway stand-in for the two routes import-assignment
// uses: the teacher module list (served twice — pre-fetch and read-back) and
// the module update. listBodies are served in order; the last one repeats.
// updateStatus/updateBody shape the POST answer. Every POST body is captured.
type assignmentStub struct {
	mu           sync.Mutex
	listBodies   []string
	listStatus   []int // parallel to listBodies; 0 means 200
	updateStatus int
	updateBody   string
	listCalls    int
	posts        []map[string]interface{}
}

func (s *assignmentStub) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v2/course/teacher/course-modules/list" && r.Method == http.MethodPost:
			i := s.listCalls
			if i >= len(s.listBodies) {
				i = len(s.listBodies) - 1
			}
			s.listCalls++
			status := http.StatusOK
			if i < len(s.listStatus) && s.listStatus[i] != 0 {
				status = s.listStatus[i]
			}
			w.WriteHeader(status)
			_, _ = w.Write([]byte(s.listBodies[i]))
		case r.URL.Path == "/api/v2/course/teacher/course-module/update" && r.Method == http.MethodPost:
			var body map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&body)
			s.posts = append(s.posts, body)
			status := s.updateStatus
			if status == 0 {
				status = http.StatusOK
			}
			w.WriteHeader(status)
			body2 := s.updateBody
			if body2 == "" {
				body2 = `{"changes":{"assignment_updated":true}}`
			}
			_, _ = w.Write([]byte(body2))
		default:
			http.Error(w, "unexpected route "+r.Method+" "+r.URL.Path, http.StatusNotFound)
		}
	}
}

func (s *assignmentStub) serve(t *testing.T) (*client.Client, string) {
	t.Helper()
	srv := httptest.NewServer(s.handler(t))
	t.Cleanup(srv.Close)
	return client.New(&config.Config{BaseURL: srv.URL, UserJWT: "test-jwt"}), srv.URL
}

// listBody renders the teacher module list with one module 101 and the given
// assignment object (nil → module has no assignment). warning adds meta.warning.
func listBody(t *testing.T, assignment map[string]interface{}, warning string) string {
	t.Helper()
	content := map[string]interface{}{
		"course_module_code": "101",
		"module_status":      "ON_CHAIN",
		"slts":               []interface{}{map[string]interface{}{"slt_index": 1, "slt_text": "Do a thing"}},
	}
	if assignment != nil {
		content["assignment"] = assignment
	}
	env := map[string]interface{}{
		"data": []interface{}{map[string]interface{}{"content": content}},
	}
	if warning != "" {
		env["meta"] = map[string]interface{}{"warning": warning}
	}
	b, err := json.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// chainOnlyListBody is the shape the merged endpoint really produces when
// db-api is down: a 206 with meta.warning and every module chain-only —
// slt_hash, course_id, source, and no `content` key at all
// (MergedCourseModuleItem.Content is omitempty and only db-api fills it).
func chainOnlyListBody(t *testing.T, warning string) string {
	t.Helper()
	env := map[string]interface{}{
		"data": []interface{}{map[string]interface{}{
			"slt_hash":  "c28e2bad6ef905179a5d81eb1ebdb9198db87f067fe867ed3c34b566d9c5f6c5",
			"course_id": "course-1",
			"source":    "chain_only",
		}},
		"meta": map[string]interface{}{"warning": warning},
	}
	b, err := json.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func docAssignment(title string) map[string]interface{} {
	return map[string]interface{}{
		"title":        title,
		"content_json": map[string]interface{}{"type": "doc", "content": []interface{}{}},
	}
}

func writeQuizFile(t *testing.T, env interface{}) string {
	t.Helper()
	b, err := json.MarshalIndent(env, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "quiz.json")
	if err := os.WriteFile(p, append(b, '\n'), 0644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestImportAssignment_PublishesVerbatimAndVerifies(t *testing.T) {
	quiz := quizEnvelope()
	existing := docAssignment("Quiz")
	existing["description"] = "Pass to continue"
	existing["image_url"] = "https://cdn/x.png"
	existing["video_url"] = "https://cdn/x.mp4"
	stored := map[string]interface{}{"title": "Quiz", "description": "Pass to continue", "image_url": "https://cdn/x.png", "video_url": "https://cdn/x.mp4", "content_json": quiz}
	stub := &assignmentStub{listBodies: []string{listBody(t, existing, ""), listBody(t, stored, "")}}
	c, _ := stub.serve(t)

	env, err := runImportAssignment(context.Background(), c, importAssignmentOptions{
		CourseID: "course-1", ModuleCode: "101", FilePath: writeQuizFile(t, quiz),
	})
	if err != nil {
		t.Fatalf("runImportAssignment: %v", err)
	}
	if !env.Verified || env.Assignment.QuestionCount != 2 || env.Assignment.PassThreshold != 2 || env.Assignment.Title != "Quiz" || env.Assignment.TitleSource != "existing" {
		t.Errorf("envelope = %+v", env)
	}
	if env.ModuleStatus != "ON_CHAIN" {
		t.Errorf("module_status = %q, want ON_CHAIN", env.ModuleStatus)
	}

	if len(stub.posts) != 1 {
		t.Fatalf("POSTs = %d, want 1", len(stub.posts))
	}
	post := stub.posts[0]
	keys := make([]string, 0, len(post))
	for k := range post {
		keys = append(keys, k)
	}
	if len(post) != 3 || post["course_id"] != "course-1" || post["course_module_code"] != "101" || post["assignment"] == nil {
		t.Errorf("POST body keys = %v, want exactly course_id, course_module_code, assignment", keys)
	}
	assign := post["assignment"].(map[string]interface{})
	if !reflect.DeepEqual(assign["content_json"], quiz) {
		t.Errorf("content_json differs from the file (structural compare)\n got: %v", assign["content_json"])
	}
	if assign["title"] != "Quiz" || assign["description"] != "Pass to continue" || assign["image_url"] != "https://cdn/x.png" || assign["video_url"] != "https://cdn/x.mp4" {
		t.Errorf("existing metadata not carried: %v", assign)
	}
	if stub.listCalls != 2 {
		t.Errorf("list calls = %d, want 2 (pre-fetch + read-back)", stub.listCalls)
	}
}

func TestImportAssignment_TitleFlagOverrides(t *testing.T) {
	quiz := quizEnvelope()
	stub := &assignmentStub{listBodies: []string{
		listBody(t, docAssignment("Old"), ""),
		listBody(t, map[string]interface{}{"title": "New", "content_json": quiz}, ""),
	}}
	c, _ := stub.serve(t)
	env, err := runImportAssignment(context.Background(), c, importAssignmentOptions{
		CourseID: "course-1", ModuleCode: "101", FilePath: writeQuizFile(t, quiz), Title: "New",
	})
	if err != nil {
		t.Fatal(err)
	}
	if env.Assignment.Title != "New" || env.Assignment.TitleSource != "flag" {
		t.Errorf("title = %q source = %q", env.Assignment.Title, env.Assignment.TitleSource)
	}
	if stub.posts[0]["assignment"].(map[string]interface{})["title"] != "New" {
		t.Error("POST did not carry the --title override")
	}
}

func TestImportAssignment_NoExistingAssignmentRequiresTitle(t *testing.T) {
	stub := &assignmentStub{listBodies: []string{listBody(t, nil, "")}}
	c, _ := stub.serve(t)
	_, err := runImportAssignment(context.Background(), c, importAssignmentOptions{
		CourseID: "course-1", ModuleCode: "101", FilePath: writeQuizFile(t, quizEnvelope()),
	})
	if err == nil || !strings.Contains(err.Error(), "title required for a module with no existing assignment") {
		t.Fatalf("err = %v", err)
	}
	if len(stub.posts) != 0 {
		t.Error("no POST may be sent when the title cannot be resolved")
	}
}

func TestImportAssignment_RejectsBeforeAnyRequest(t *testing.T) {
	cases := []struct {
		name string
		file interface{}
		want string
	}{
		{"tiptap doc", map[string]interface{}{"type": "doc", "content": []interface{}{}}, "course import"},
		{"quiz-evidence", map[string]interface{}{"type": "quiz-evidence", "version": 1}, "not a quiz"},
		{"array", []interface{}{1, 2}, "not a quiz"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub := &assignmentStub{listBodies: []string{listBody(t, docAssignment("Quiz"), "")}}
			c, _ := stub.serve(t)
			_, err := runImportAssignment(context.Background(), c, importAssignmentOptions{
				CourseID: "course-1", ModuleCode: "101", FilePath: writeQuizFile(t, tc.file),
			})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want mention of %q", err, tc.want)
			}
			if stub.listCalls != 0 || len(stub.posts) != 0 {
				t.Error("a rejected file must not cause any request")
			}
		})
	}
}

func TestImportAssignment_InvalidQuizListsEveryRule(t *testing.T) {
	broken := quizEnvelope()
	broken["passThreshold"] = float64(9)
	qs := broken["questions"].([]interface{})
	qs[1].(map[string]interface{})["correctValue"] = "zzz"
	qs[0].(map[string]interface{})["prompt"] = ""
	stub := &assignmentStub{listBodies: []string{listBody(t, docAssignment("Quiz"), "")}}
	c, _ := stub.serve(t)
	_, err := runImportAssignment(context.Background(), c, importAssignmentOptions{
		CourseID: "course-1", ModuleCode: "101", FilePath: writeQuizFile(t, broken),
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
	for _, want := range []string{"threshold-exceeds-questions", "dangling-correct-value", "malformed-prompt"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should list %q: %v", want, err)
		}
	}
	if stub.listCalls != 0 {
		t.Error("validation must run before any request")
	}
}

func TestImportAssignment_DryRunSendsNothing(t *testing.T) {
	stub := &assignmentStub{listBodies: []string{listBody(t, docAssignment("Quiz"), "")}}
	c, _ := stub.serve(t)
	env, err := runImportAssignment(context.Background(), c, importAssignmentOptions{
		CourseID: "course-1", ModuleCode: "101", FilePath: writeQuizFile(t, quizEnvelope()), DryRun: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !env.DryRun || env.Verified {
		t.Errorf("dry run envelope = %+v, want dry_run true, verified false", env)
	}
	if len(stub.posts) != 0 || stub.listCalls != 1 {
		t.Errorf("dry run must fetch once and POST never; list=%d posts=%d", stub.listCalls, len(stub.posts))
	}
}

func TestImportAssignment_ReadBackMismatchIsVerifyError(t *testing.T) {
	quiz := quizEnvelope()
	stub := &assignmentStub{listBodies: []string{
		listBody(t, docAssignment("Quiz"), ""),
		listBody(t, docAssignment("Quiz"), ""), // stored value unchanged
	}}
	c, _ := stub.serve(t)
	_, err := runImportAssignment(context.Background(), c, importAssignmentOptions{
		CourseID: "course-1", ModuleCode: "101", FilePath: writeQuizFile(t, quiz),
	})
	var verify *apierr.VerifyError
	if !errors.As(err, &verify) {
		t.Fatalf("err = %v, want VerifyError", err)
	}
	if apierr.Kind(err) != apierr.KindVerify || !strings.Contains(err.Error(), "accepted") {
		t.Errorf("kind = %q, err = %v", apierr.Kind(err), err)
	}
	if len(stub.posts) != 1 {
		t.Error("the update must have been sent before the mismatch was detected")
	}
}

func TestImportAssignment_ReadBackMetadataMismatchIsVerifyError(t *testing.T) {
	quiz := quizEnvelope()
	stub := &assignmentStub{listBodies: []string{
		listBody(t, docAssignment("Quiz"), ""),
		listBody(t, map[string]interface{}{"title": "Something Else", "content_json": quiz}, ""),
	}}
	c, _ := stub.serve(t)
	_, err := runImportAssignment(context.Background(), c, importAssignmentOptions{
		CourseID: "course-1", ModuleCode: "101", FilePath: writeQuizFile(t, quiz),
	})
	if apierr.Kind(err) != apierr.KindVerify || !strings.Contains(err.Error(), "title") {
		t.Fatalf("err = %v, want verify error naming title", err)
	}
}

func TestImportAssignment_DegradedPreFetchIsErrorNotTitleError(t *testing.T) {
	stub := &assignmentStub{
		listBodies: []string{chainOnlyListBody(t, "DB API unavailable, showing on-chain data only")},
		listStatus: []int{http.StatusPartialContent},
	}
	c, _ := stub.serve(t)
	_, err := runImportAssignment(context.Background(), c, importAssignmentOptions{
		CourseID: "course-1", ModuleCode: "101", FilePath: writeQuizFile(t, quizEnvelope()),
	})
	if err == nil || !strings.Contains(err.Error(), "DB API unavailable") || strings.Contains(err.Error(), "title required") {
		t.Fatalf("err = %v, want an error naming the warning, not the title error", err)
	}
	if apierr.Kind(err) == apierr.KindNotFound {
		t.Errorf("a degraded read must not classify as not_found: %v", err)
	}
	if len(stub.posts) != 0 {
		t.Error("no POST may follow a degraded pre-fetch")
	}
}

// An Andamioscan outage also produces a 206 with meta.warning, but a module
// that came back with content came from db-api and is complete: publishing
// must proceed and verify normally.
func TestImportAssignment_AndamioscanOnlyWarningStillPublishes(t *testing.T) {
	quiz := quizEnvelope()
	stub := &assignmentStub{
		listBodies: []string{
			listBody(t, docAssignment("Quiz"), "Andamioscan unavailable, showing database data only"),
			listBody(t, map[string]interface{}{"title": "Quiz", "content_json": quiz}, "Andamioscan unavailable, showing database data only"),
		},
		listStatus: []int{http.StatusPartialContent, http.StatusPartialContent},
	}
	c, _ := stub.serve(t)
	env, err := runImportAssignment(context.Background(), c, importAssignmentOptions{
		CourseID: "course-1", ModuleCode: "101", FilePath: writeQuizFile(t, quiz),
	})
	if err != nil {
		t.Fatalf("a found module with content is complete regardless of the warning: %v", err)
	}
	if !env.Verified || len(stub.posts) != 1 {
		t.Errorf("verified=%v posts=%d, want a verified publish", env.Verified, len(stub.posts))
	}
}

func TestImportAssignment_ModuleNotInListIsNotFound(t *testing.T) {
	stub := &assignmentStub{listBodies: []string{`{"data":[{"content":{"course_module_code":"999","module_status":"DRAFT"}}]}`}}
	c, _ := stub.serve(t)
	_, err := runImportAssignment(context.Background(), c, importAssignmentOptions{
		CourseID: "course-1", ModuleCode: "101", FilePath: writeQuizFile(t, quizEnvelope()),
	})
	if apierr.Kind(err) != apierr.KindNotFound {
		t.Fatalf("kind = %q, want not_found; err = %v", apierr.Kind(err), err)
	}
	if len(stub.posts) != 0 {
		t.Error("no POST for an unknown module")
	}
}

func TestImportAssignment_ReadBackOmittingModuleSaysAccepted(t *testing.T) {
	stub := &assignmentStub{listBodies: []string{
		listBody(t, docAssignment("Quiz"), ""),
		`{"data":[{"content":{"course_module_code":"999","module_status":"DRAFT"}}]}`,
	}}
	c, _ := stub.serve(t)
	_, err := runImportAssignment(context.Background(), c, importAssignmentOptions{
		CourseID: "course-1", ModuleCode: "101", FilePath: writeQuizFile(t, quizEnvelope()),
	})
	if err == nil || !strings.Contains(err.Error(), "accepted") {
		t.Fatalf("err = %v, want a message saying the update was accepted", err)
	}
	if apierr.Kind(err) == apierr.KindVerify {
		t.Errorf("a healthy list that omits the module is not a degraded read: %v", err)
	}
}

func TestImportAssignment_DegradedReadBackIsVerifyErrorNamingWarning(t *testing.T) {
	stub := &assignmentStub{
		listBodies: []string{listBody(t, docAssignment("Quiz"), ""), chainOnlyListBody(t, "DB API unavailable, showing on-chain data only")},
		listStatus: []int{0, http.StatusPartialContent},
	}
	c, _ := stub.serve(t)
	_, err := runImportAssignment(context.Background(), c, importAssignmentOptions{
		CourseID: "course-1", ModuleCode: "101", FilePath: writeQuizFile(t, quizEnvelope()),
	})
	if apierr.Kind(err) != apierr.KindVerify {
		t.Fatalf("kind = %q, err = %v", apierr.Kind(err), err)
	}
	if !strings.Contains(err.Error(), "DB API unavailable") || strings.Contains(err.Error(), "did not read back identical") {
		t.Errorf("degraded read-back must name the warning, not claim a mismatch: %v", err)
	}
}

func TestImportAssignment_ReadBackFailureKeepsUnderlyingKind(t *testing.T) {
	stub := &assignmentStub{
		listBodies: []string{listBody(t, docAssignment("Quiz"), ""), `{"message":"down"}`},
		listStatus: []int{0, http.StatusServiceUnavailable},
	}
	c, _ := stub.serve(t)
	_, err := runImportAssignment(context.Background(), c, importAssignmentOptions{
		CourseID: "course-1", ModuleCode: "101", FilePath: writeQuizFile(t, quizEnvelope()),
	})
	if apierr.Kind(err) != apierr.KindServer {
		t.Fatalf("kind = %q, want server; err = %v", apierr.Kind(err), err)
	}
	if !strings.Contains(err.Error(), "accepted") {
		t.Errorf("message must say the update was accepted: %v", err)
	}
}

func TestImportAssignment_UpdateErrorsKeepTheirKind(t *testing.T) {
	for _, tc := range []struct {
		status int
		kind   string
	}{{http.StatusNotFound, apierr.KindNotFound}, {http.StatusUnauthorized, apierr.KindAuth}} {
		stub := &assignmentStub{listBodies: []string{listBody(t, docAssignment("Quiz"), "")}, updateStatus: tc.status, updateBody: `{"message":"stub"}`}
		c, _ := stub.serve(t)
		_, err := runImportAssignment(context.Background(), c, importAssignmentOptions{
			CourseID: "course-1", ModuleCode: "101", FilePath: writeQuizFile(t, quizEnvelope()),
		})
		if apierr.Kind(err) != tc.kind {
			t.Errorf("status %d: kind = %q, want %q (%v)", tc.status, apierr.Kind(err), tc.kind, err)
		}
	}
}

// End-to-end through the binary: the JSON error document for a read-back
// mismatch is exactly one document carrying kind verify, exit 1, and the
// dry-run success envelope has the documented shape.
func TestImportAssignment_CLIJSONContract(t *testing.T) {
	bin := buildTestBinary(t)
	quiz := quizEnvelope()
	file := writeQuizFile(t, quiz)

	t.Run("verify mismatch", func(t *testing.T) {
		stub := &assignmentStub{listBodies: []string{listBody(t, docAssignment("Quiz"), ""), listBody(t, docAssignment("Quiz"), "")}}
		_, url := stub.serve(t)
		stdout, stderr, code := runCLI(t, bin, url, "course", "import-assignment", "course-1", "101", file, "--output", "json")
		if code != 1 {
			t.Errorf("exit = %d, want 1", code)
		}
		var parsed map[string]string
		if err := json.Unmarshal([]byte(stdout), &parsed); err != nil {
			t.Fatalf("stdout must be exactly one JSON document: %v\n%q", err, stdout)
		}
		if parsed["kind"] != "verify" {
			t.Errorf("kind = %q, want verify (stderr %q)", parsed["kind"], stderr)
		}
	})

	t.Run("dry run envelope", func(t *testing.T) {
		stub := &assignmentStub{listBodies: []string{listBody(t, docAssignment("Quiz"), "")}}
		_, url := stub.serve(t)
		stdout, _, code := runCLI(t, bin, url, "course", "import-assignment", "course-1", "101", file, "--dry-run", "--output", "json")
		if code != 0 {
			t.Fatalf("exit = %d, want 0; stdout %q", code, stdout)
		}
		var env ImportAssignmentEnvelope
		if err := json.Unmarshal([]byte(stdout), &env); err != nil {
			t.Fatalf("stdout is not the envelope: %v\n%q", err, stdout)
		}
		if !env.DryRun || env.Verified || env.Assignment.QuestionCount != 2 || env.CourseID != "course-1" || env.ModuleCode != "101" {
			t.Errorf("envelope = %+v", env)
		}
		if len(stub.posts) != 0 {
			t.Error("dry run sent a POST")
		}
	})

	t.Run("text dry run summary on stdout, payload on stderr", func(t *testing.T) {
		stub := &assignmentStub{listBodies: []string{listBody(t, docAssignment("Quiz"), "")}}
		_, url := stub.serve(t)
		stdout, stderr, code := runCLI(t, bin, url, "course", "import-assignment", "course-1", "101", file, "--dry-run", "--show-payload")
		if code != 0 {
			t.Fatalf("exit = %d; stderr %q", code, stderr)
		}
		if !strings.Contains(stdout, "2 questions") || !strings.Contains(stdout, "threshold 2") {
			t.Errorf("stdout summary = %q", stdout)
		}
		if !strings.Contains(stderr, `"course_module_code"`) {
			t.Errorf("--show-payload should print the payload on stderr: %q", stderr)
		}
		if strings.Contains(stdout, `"course_module_code"`) {
			t.Error("payload leaked to stdout")
		}
	})

	t.Run("missing file", func(t *testing.T) {
		stub := &assignmentStub{listBodies: []string{listBody(t, docAssignment("Quiz"), "")}}
		_, url := stub.serve(t)
		_, stderr, code := runCLI(t, bin, url, "course", "import-assignment", "course-1", "101", filepath.Join(t.TempDir(), "nope.json"))
		if code != 1 || !strings.Contains(stderr, "nope.json") {
			t.Errorf("exit = %d, stderr = %q", code, stderr)
		}
		if stub.listCalls != 0 {
			t.Error("a missing file must not cause a request")
		}
	})
}
