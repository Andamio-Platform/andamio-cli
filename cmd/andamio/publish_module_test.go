package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Andamio-Platform/andamio-cli/internal/apierr"
	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/Andamio-Platform/andamio-cli/internal/config"
)

// The publish response is api_types.CourseModuleEntity — module_status,
// slt_hash, is_live — never `source`. The old check keyed on
// `source == "merged"`, so the "may not have been linked" warning fired on
// every publish, including this exact ON_CHAIN response captured on preprod
// (#158).
const publishedOnChainBody = `{
  "course_module_code": "100",
  "course_id": "68d247d3",
  "is_live": true,
  "module_status": "ON_CHAIN",
  "slt_hash": "c28e2bad6ef905179a5d81eb1ebdb9198db87f067fe867ed3c34b566d9c5f6c5",
  "title": "Module 100"
}`

func stubPublishServer(t *testing.T, status int, body string) (*client.Client, *map[string]interface{}) {
	t.Helper()
	var captured map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/course/teacher/course-module/publish" || r.Method != http.MethodPost {
			http.Error(w, "unexpected route "+r.Method+" "+r.URL.Path, http.StatusNotFound)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&captured)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return client.New(&config.Config{BaseURL: srv.URL, UserJWT: "test-jwt"}), &captured
}

func TestPublishedModuleIsLinked(t *testing.T) {
	cases := []struct {
		name string
		body string
		want bool
	}{
		{"ON_CHAIN with hash", publishedOnChainBody, true},
		{"ON_CHAIN without hash", `{"module_status":"ON_CHAIN"}`, true},
		{"hash without status", `{"slt_hash":"abc"}`, true},
		{"lowercase on_chain", `{"module_status":"on_chain"}`, true},
		{"nested content shape", `{"content":{"module_status":"ON_CHAIN"}}`, true},
		{"DRAFT no hash", `{"module_status":"DRAFT","slt_hash":""}`, false},
		{"APPROVED no hash", `{"module_status":"APPROVED"}`, false},
		{"empty body", `{}`, false},
		{"whitespace hash", `{"slt_hash":"   "}`, false},
		{"source merged alone is not a signal", `{"source":"merged"}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var resp map[string]interface{}
			if err := json.Unmarshal([]byte(tc.body), &resp); err != nil {
				t.Fatal(err)
			}
			if got := publishedModuleIsLinked(resp); got != tc.want {
				t.Errorf("publishedModuleIsLinked = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPublishModuleFlow_LinkedModule_NoWarning(t *testing.T) {
	c, captured := stubPublishServer(t, http.StatusOK, publishedOnChainBody)
	var stderr bytes.Buffer

	resp, err := publishModuleFlow(context.Background(), c, "68d247d3", "100", &stderr)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if (*captured)["course_id"] != "68d247d3" || (*captured)["course_module_code"] != "100" {
		t.Errorf("request payload = %v", *captured)
	}
	if resp["module_status"] != "ON_CHAIN" {
		t.Errorf("response not passed through: %v", resp)
	}
	if strings.Contains(stderr.String(), "may not have been linked") {
		t.Errorf("warning fired for an ON_CHAIN module with slt_hash:\n%s", stderr.String())
	}
	if strings.Contains(stderr.String(), "register-module") {
		t.Errorf("register-module hint printed for a linked module:\n%s", stderr.String())
	}
}

func TestPublishModuleFlow_UnlinkedModule_Warns(t *testing.T) {
	c, _ := stubPublishServer(t, http.StatusOK, `{"course_module_code":"100","module_status":"APPROVED","slt_hash":"","is_live":true}`)
	var stderr bytes.Buffer

	if _, err := publishModuleFlow(context.Background(), c, "68d247d3", "100", &stderr); err != nil {
		t.Fatalf("publish: %v", err)
	}
	out := stderr.String()
	if !strings.Contains(out, "module 100 may not have been linked") {
		t.Errorf("expected the unlinked warning, got:\n%s", out)
	}
	if !strings.Contains(out, "register-module --course-id 68d247d3 --module-code 100") {
		t.Errorf("expected the register-module hint, got:\n%s", out)
	}
}

func TestPublishModuleFlow_GatewayWarningEchoed(t *testing.T) {
	c, _ := stubPublishServer(t, http.StatusOK, `{"module_status":"ON_CHAIN","slt_hash":"abc","warning":"already live"}`)
	var stderr bytes.Buffer

	if _, err := publishModuleFlow(context.Background(), c, "c", "m", &stderr); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr.String(), "Warning: already live") {
		t.Errorf("gateway warning not echoed: %q", stderr.String())
	}
	if strings.Contains(stderr.String(), "may not have been linked") {
		t.Errorf("linked module must not get the unlinked warning: %q", stderr.String())
	}
}

func TestPublishModuleFlow_ErrorStaysTyped(t *testing.T) {
	c, _ := stubPublishServer(t, http.StatusNotFound, `{"error":{"code":"NOT_FOUND","message":"module not found"}}`)
	var stderr bytes.Buffer

	_, err := publishModuleFlow(context.Background(), c, "c", "m", &stderr)
	if apierr.Kind(err) != apierr.KindNotFound {
		t.Errorf("Kind = %q, want not_found (%v)", apierr.Kind(err), err)
	}
	if stderr.Len() != 0 {
		t.Errorf("no warnings on a failed publish, got %q", stderr.String())
	}
}

// create-module --approve computes the slt_hash it sends; the JSON result
// must echo it so the caller does not need a follow-up read (#158).
func TestCreateModuleResult_EchoesSltHashOnlyWhenApproved(t *testing.T) {
	approved := CreateModuleResult{CourseID: "c", ModuleCode: "100", Title: "T", SltHash: "deadbeef", ModuleStatus: "APPROVED"}
	b, _ := json.Marshal(approved)
	if !strings.Contains(string(b), `"slt_hash":"deadbeef"`) || !strings.Contains(string(b), `"module_status":"APPROVED"`) {
		t.Errorf("approved result missing slt_hash/module_status: %s", b)
	}

	plain := CreateModuleResult{CourseID: "c", ModuleCode: "100", Title: "T"}
	b, _ = json.Marshal(plain)
	if strings.Contains(string(b), "slt_hash") || strings.Contains(string(b), "module_status") {
		t.Errorf("unapproved result must omit slt_hash/module_status: %s", b)
	}
}
