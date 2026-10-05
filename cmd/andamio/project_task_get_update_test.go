package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// A draft task has no description, reward tokens or hash yet, so the text
// view shows only the five task list columns.
func TestRenderTaskDetail_DraftTaskShowsFiveFields(t *testing.T) {
	item := map[string]interface{}{
		"task_index":      float64(2),
		"task_status":     "DRAFT",
		"lovelace_amount": float64(5000000),
		"expiration":      "2026-11-01T00:00:00Z",
		"content":         map[string]interface{}{"title": "Write the guide"},
	}

	var buf bytes.Buffer
	renderTaskDetail(&buf, item)

	want := "Index:       2\n" +
		"Title:       Write the guide\n" +
		"Status:      DRAFT\n" +
		"Lovelace:    5000000\n" +
		"Expiration:  2026-11-01T00:00:00Z\n"
	if got := buf.String(); got != want {
		t.Errorf("output =\n%s\nwant\n%s", got, want)
	}
}

// An on-chain task adds description, one line per reward token and the hash.
// Field names follow MergedTaskListItem in the gateway's swagger spec.
func TestRenderTaskDetail_OnChainTaskShowsDescriptionTokensHash(t *testing.T) {
	item := map[string]interface{}{
		"task_index":      float64(3),
		"task_status":     "ON_CHAIN",
		"lovelace_amount": float64(5000000),
		"expiration":      "2026-11-01T00:00:00Z",
		"task_hash":       "9f2ce41a",
		"content": map[string]interface{}{
			"title":        "Write the guide",
			"description":  "One page for new contributors.",
			"content_json": map[string]interface{}{"type": "doc"},
		},
		"assets": []interface{}{
			map[string]interface{}{"policy_id": testPolicyA, "name": "AndamioToken", "amount": "2"},
			map[string]interface{}{"policy_id": testPolicyB, "name": "Badge", "amount": "1"},
		},
	}

	var buf bytes.Buffer
	renderTaskDetail(&buf, item)

	want := "Index:       3\n" +
		"Title:       Write the guide\n" +
		"Status:      ON_CHAIN\n" +
		"Lovelace:    5000000\n" +
		"Expiration:  2026-11-01T00:00:00Z\n" +
		"Description: One page for new contributors.\n" +
		"Tokens:      2 × AndamioToken (" + testPolicyA + ")\n" +
		"             1 × Badge (" + testPolicyB + ")\n" +
		"Task hash:   9f2ce41a\n"
	if got := buf.String(); got != want {
		t.Errorf("output =\n%s\nwant\n%s", got, want)
	}
}

// task get prints the text view by default and the full task object with
// --output json.
func TestTaskGet_RendersByOutputFormat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/project/manager/tasks/list" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"task_index":3,"task_status":"ON_CHAIN","lovelace_amount":5000000,` +
			`"task_hash":"9f2ce41a","content":{"title":"Write the guide","description":"One page."}}]}`))
	}))
	t.Cleanup(srv.Close)

	bin := buildTestBinary(t)
	jwt := jwtWithExp(time.Now().Add(time.Hour))

	stdout, stderr, code := runCLIWithJWT(t, bin, srv.URL, jwt, nil,
		"project", "task", "get", "3", "--project-id", "proj-1")
	if code != 0 {
		t.Fatalf("text mode exit %d, stderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "Title:       Write the guide\n") || !strings.Contains(stdout, "Task hash:   9f2ce41a\n") {
		t.Errorf("text mode stdout missing detail lines:\n%s", stdout)
	}
	if json.Valid([]byte(stdout)) {
		t.Errorf("text mode stdout is JSON, want the text view:\n%s", stdout)
	}

	stdout, stderr, code = runCLIWithJWT(t, bin, srv.URL, jwt, nil,
		"project", "task", "get", "3", "--project-id", "proj-1", "--output", "json")
	if code != 0 {
		t.Fatalf("json mode exit %d, stderr: %s", code, stderr)
	}
	var task struct {
		TaskHash string `json:"task_hash"`
		Content  struct {
			Title       string `json:"title"`
			Description string `json:"description"`
		} `json:"content"`
	}
	if err := json.Unmarshal([]byte(stdout), &task); err != nil {
		t.Fatalf("json mode stdout is not JSON: %v\n%s", err, stdout)
	}
	if task.Content.Title != "Write the guide" || task.Content.Description != "One page." || task.TaskHash != "9f2ce41a" {
		t.Errorf("json mode lost fields: %+v", task)
	}
}

// task update with no field flags errors before sending anything, instead of
// posting an empty update.
func TestTaskUpdate_NoFieldFlagsErrorsBeforeRequest(t *testing.T) {
	var hits int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&hits, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	t.Cleanup(srv.Close)

	bin := buildTestBinary(t)
	_, stderr, code := runCLIWithJWT(t, bin, srv.URL, jwtWithExp(time.Now().Add(time.Hour)), nil,
		"project", "task", "update", "0", "--project-id", "proj-1")
	if code == 0 {
		t.Fatal("exit 0, want an error")
	}
	if !strings.Contains(stderr, "no fields to update") {
		t.Errorf("stderr = %q, want the no-fields error", stderr)
	}
	if n := atomic.LoadInt64(&hits); n != 0 {
		t.Errorf("%d request(s) reached the server, want 0", n)
	}
}
