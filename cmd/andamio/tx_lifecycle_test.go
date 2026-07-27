package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/Andamio-Platform/andamio-cli/internal/config"
)

// pathRecorder serves a fixed JSON body for every request and records the paths
// it was asked for, so a test can assert on which routes the CLI touched — not
// just on the value it came back with.
type pathRecorder struct {
	mu    sync.Mutex
	paths []string
	body  interface{}
}

func (p *pathRecorder) handler(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	p.paths = append(p.paths, r.URL.Path)
	p.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(p.body)
}

func (p *pathRecorder) requested() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string{}, p.paths...)
}

// newRoutingClient serves a different body per request path, so a test can
// distinguish which of two lookups produced an answer — the difference between
// "returned the right hash" and "returned the right hash for the right reason".
func newRoutingClient(t *testing.T, rec *pathRecorder, byPath map[string]interface{}) *client.Client {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.mu.Lock()
		rec.paths = append(rec.paths, r.URL.Path)
		rec.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		if body, ok := byPath[r.URL.Path]; ok {
			_ = json.NewEncoder(w).Encode(body)
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"no stub for this path"}`))
	}))
	t.Cleanup(srv.Close)

	return client.New(&config.Config{BaseURL: srv.URL})
}

func newRecordingClient(t *testing.T, body interface{}) (*client.Client, *pathRecorder) {
	t.Helper()

	rec := &pathRecorder{body: body}
	srv := httptest.NewServer(http.HandlerFunc(rec.handler))
	t.Cleanup(srv.Close)

	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".andamio")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	b, _ := json.Marshal(map[string]string{"base_url": srv.URL, "api_key": "test-key"})
	if err := os.WriteFile(filepath.Join(dir, "config.json"), b, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	return client.New(cfg), rec
}

func taskListBody(taskHash string) map[string]interface{} {
	return map[string]interface{}{
		"data": []interface{}{
			map[string]interface{}{"task_index": float64(0), "task_hash": taskHash},
		},
	}
}

// The credential-claim lookup MUST consult the contributor's commitments, and
// must prefer that answer over the task-list fallback.
//
// 1.0 retired the `project contributor` commands, not the gateway routes, and
// tx run is generic — project_credential_claim still runs through it. During
// implementation this lookup was removed on the mistaken assumption that the
// route was going away too, which silently degraded the path to the fallback
// below. On a multi-task project that returns the wrong task's hash, so the
// gateway confirms the claim against a task the contributor never completed.
//
// The two tasks here carry different hashes precisely so a fallback-only
// implementation cannot pass.
func TestExtractTaskHash_CredentialClaim_PrefersContributorCommitment(t *testing.T) {
	commitments := []interface{}{
		map[string]interface{}{
			"project_id": "proj-1",
			"task_hash":  "committed-task-hash",
			"content":    map[string]interface{}{"commitment_status": "ACCEPTED"},
		},
	}

	rec := &pathRecorder{}
	c := newRoutingClient(t, rec, map[string]interface{}{
		"/api/v2/project/contributor/commitments/list": commitments,
		"/api/v2/project/user/tasks/list":              taskListBody("first-task-hash"),
	})

	body := map[string]interface{}{"project_id": "proj-1"}
	got := extractTaskHash(t.Context(), body, "project_credential_claim", c)

	if got == "first-task-hash" {
		t.Error("fell through to the task-list fallback; the accepted commitment's hash must win")
	}
	if got != "committed-task-hash" {
		t.Errorf("task hash = %q, want %q", got, "committed-task-hash")
	}

	var sawCommitments bool
	for _, path := range rec.requested() {
		if strings.Contains(path, "/project/contributor/commitments/list") {
			sawCommitments = true
		}
	}
	if !sawCommitments {
		t.Error("never queried the contributor commitments route")
	}
}

// Only when the commitments lookup yields nothing does the task list stand in.
func TestExtractTaskHash_CredentialClaim_FallsBackWhenNoCommitmentMatches(t *testing.T) {
	rec := &pathRecorder{}
	c := newRoutingClient(t, rec, map[string]interface{}{
		"/api/v2/project/contributor/commitments/list": []interface{}{},
		"/api/v2/project/user/tasks/list":              taskListBody("first-task-hash"),
	})

	body := map[string]interface{}{"project_id": "proj-1"}
	if got := extractTaskHash(t.Context(), body, "project_credential_claim", c); got != "first-task-hash" {
		t.Errorf("task hash = %q, want the task-list fallback %q", got, "first-task-hash")
	}
}

// A commitment for a different project must not be borrowed.
func TestExtractTaskHash_CredentialClaim_IgnoresOtherProjects(t *testing.T) {
	commitments := []interface{}{
		map[string]interface{}{
			"project_id": "some-other-project",
			"task_hash":  "wrong-project-hash",
			"content":    map[string]interface{}{"commitment_status": "ACCEPTED"},
		},
	}

	rec := &pathRecorder{}
	c := newRoutingClient(t, rec, map[string]interface{}{
		"/api/v2/project/contributor/commitments/list": commitments,
		"/api/v2/project/user/tasks/list":              taskListBody("first-task-hash"),
	})

	body := map[string]interface{}{"project_id": "proj-1"}
	if got := extractTaskHash(t.Context(), body, "project_credential_claim", c); got == "wrong-project-hash" {
		t.Error("used a commitment belonging to a different project")
	}
}

func TestExtractTaskHash_CredentialClaim_EmptyTaskList(t *testing.T) {
	c, _ := newRecordingClient(t, map[string]interface{}{"data": []interface{}{}})

	body := map[string]interface{}{"project_id": "proj-1"}
	if got := extractTaskHash(t.Context(), body, "project_credential_claim", c); got != "" {
		t.Errorf("task hash = %q, want empty string for an empty task list", got)
	}
}

func TestExtractTaskHash_CredentialClaim_MissingProjectID(t *testing.T) {
	c, rec := newRecordingClient(t, taskListBody("abc123"))

	if got := extractTaskHash(t.Context(), map[string]interface{}{}, "project_credential_claim", c); got != "" {
		t.Errorf("task hash = %q, want empty string when project_id is absent", got)
	}
	if n := len(rec.requested()); n != 0 {
		t.Errorf("made %d request(s) without a project_id; should short-circuit", n)
	}
}

func TestExtractTaskHash_CredentialClaim_NilClient(t *testing.T) {
	body := map[string]interface{}{"project_id": "proj-1"}
	if got := extractTaskHash(t.Context(), body, "project_credential_claim", nil); got != "" {
		t.Errorf("task hash = %q, want empty string with a nil client", got)
	}
}

// Unchanged-behavior guard: the body-carried types must keep reading task_hash
// straight from the request and must not make a lookup call at all.
func TestExtractTaskHash_BodyCarriedTypes(t *testing.T) {
	for _, txType := range []string{"project_join", "task_submit", "task_assess"} {
		t.Run(txType, func(t *testing.T) {
			c, rec := newRecordingClient(t, taskListBody("from-lookup"))

			body := map[string]interface{}{"task_hash": "from-body", "project_id": "proj-1"}
			if got := extractTaskHash(t.Context(), body, txType, c); got != "from-body" {
				t.Errorf("task hash = %q, want %q", got, "from-body")
			}
			if n := len(rec.requested()); n != 0 {
				t.Errorf("made %d request(s); %s carries task_hash in the body", n, txType)
			}
		})
	}
}

func TestExtractTaskHash_UnknownTypeAndMalformedBody(t *testing.T) {
	c, _ := newRecordingClient(t, taskListBody("abc123"))

	if got := extractTaskHash(t.Context(), map[string]interface{}{"task_hash": "x"}, "course_create", c); got != "" {
		t.Errorf("task hash = %q, want empty string for an unrelated tx type", got)
	}
	if got := extractTaskHash(t.Context(), "not-a-map", "project_join", c); got != "" {
		t.Errorf("task hash = %q, want empty string for a non-map body", got)
	}
}
