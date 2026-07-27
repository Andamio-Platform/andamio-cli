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

// The credential-claim lookup must not touch the contributor surface retired in
// 1.0. Asserting on the requested paths — rather than only on the returned hash
// — is what makes this a real guard: a reintroduced contributor call that
// happened to also return the right hash would pass a value-only assertion.
func TestExtractTaskHash_CredentialClaim_UsesNoRetiredRoute(t *testing.T) {
	c, rec := newRecordingClient(t, taskListBody("abc123"))

	body := map[string]interface{}{"project_id": "proj-1"}
	got := extractTaskHash(t.Context(), body, "project_credential_claim", c)

	if got != "abc123" {
		t.Errorf("task hash = %q, want %q", got, "abc123")
	}
	for _, path := range rec.requested() {
		if strings.Contains(path, "/contributor/") || strings.Contains(path, "/student/") {
			t.Errorf("called retired route %q", path)
		}
	}
	if len(rec.requested()) == 0 {
		t.Error("no request was made; the fallback lookup did not run")
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
