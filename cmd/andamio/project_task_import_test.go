package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// If the existing-tasks fetch fails, import stops instead of sending updates
// without the DRAFT status check. Covers --dry-run too, which shares the fetch.
func TestTaskImport_TaskListFetchFailureStopsImport(t *testing.T) {
	var writes int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v2/project/manager/projects/list":
			_, _ = w.Write([]byte(`{"data":[{"project_id":"proj-1","contributor_state_id":"policy-1","content":{"title":"Demo Project"}}]}`))
		case "/api/v2/project/manager/tasks/list":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"status_code":500,"message":"db-api unavailable"}`))
		default:
			atomic.AddInt64(&writes, 1)
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	t.Cleanup(srv.Close)

	workdir := t.TempDir()
	taskDir := filepath.Join(workdir, "tasks", "demo-project")
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	md := "---\ntitle: Live task\nlovelace: \"5000000\"\nexpiration_time: \"2027-01-01\"\nindex: 0\n---\n\nBody.\n"
	if err := os.WriteFile(filepath.Join(taskDir, "task-0.md"), []byte(md), 0o644); err != nil {
		t.Fatalf("write task file: %v", err)
	}

	bin := buildTestBinary(t)
	jwt := jwtWithExp(time.Now().Add(time.Hour))

	for _, extra := range [][]string{nil, {"--dry-run"}} {
		args := append([]string{"project", "task", "import", "proj-1"}, extra...)
		_, stderr, code := runCLIInDir(t, bin, srv.URL, jwt, workdir, nil, args...)
		if code == 0 {
			t.Fatalf("%v: exit 0, want an error", extra)
		}
		if !strings.Contains(stderr, "needed for the DRAFT status check") {
			t.Errorf("%v: stderr = %q, want the task-list fetch error", extra, stderr)
		}
	}
	if n := atomic.LoadInt64(&writes); n != 0 {
		t.Errorf("%d create/update request(s) reached the server, want 0", n)
	}
}

// create and import read the contributor state ID straight off the resolved
// project, so a project that isn't on-chain yet must still error before any
// write is sent.
func TestTaskCreateImport_ProjectNotOnChainErrorsBeforeWrite(t *testing.T) {
	var writes int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v2/project/manager/projects/list":
			_, _ = w.Write([]byte(`{"data":[{"project_id":"proj-1","contributor_state_id":"","content":{"title":"Demo Project"}}]}`))
		case "/api/v2/project/manager/tasks/list":
			_, _ = w.Write([]byte(`{"data":[]}`))
		default:
			atomic.AddInt64(&writes, 1)
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	t.Cleanup(srv.Close)

	bin := buildTestBinary(t)
	jwt := jwtWithExp(time.Now().Add(time.Hour))

	cases := map[string][]string{
		"create": {"project", "task", "create", "proj-1", "--title", "T", "--lovelace", "5000000", "--expiration", "2027-01-01T00:00:00Z"},
		"import": {"project", "task", "import", "proj-1"},
	}
	for name, args := range cases {
		_, stderr, code := runCLIInDir(t, bin, srv.URL, jwt, t.TempDir(), nil, args...)
		if code == 0 {
			t.Fatalf("%s: exit 0, want an error", name)
		}
		if !strings.Contains(stderr, "has no contributor_state_id (may not be on-chain yet)") {
			t.Errorf("%s: stderr = %q, want the not-on-chain error", name, stderr)
		}
	}
	if n := atomic.LoadInt64(&writes); n != 0 {
		t.Errorf("%d create/update request(s) reached the server, want 0", n)
	}
}
