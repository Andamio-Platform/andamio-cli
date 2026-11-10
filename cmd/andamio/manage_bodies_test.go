package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/Andamio-Platform/andamio-cli/internal/cardano"
)

// course teacher mint-modules and project task mint build the modules_manage
// and tasks_manage bodies that 'tx run' used to need by hand (cli#215). Two
// things must never regress, and neither fails loudly on its own:
//
//   - Every key of ManageModulesTxRequest / ManageTasksTxRequest is sent,
//     empty lists as []. The gateway answers 400 to a missing key even when
//     the list it would carry is empty.
//   - A module's SLT strings reach the body byte for byte as the DB record
//     holds them, so the minted module's hash is the hash on its record.

// marshalToMap round-trips a body through JSON, so assertions see the wire
// shape rather than the Go struct.
func marshalToMap(t *testing.T, v interface{}) map[string]interface{} {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return m
}

// requireEmptyList fails unless key is present and is an empty JSON array
// (not null, not absent).
func requireEmptyList(t *testing.T, m map[string]interface{}, key string) {
	t.Helper()
	v, present := m[key]
	if !present {
		t.Fatalf("%s missing from body; the gateway refuses a body without it", key)
	}
	list, ok := v.([]interface{})
	if !ok {
		t.Fatalf("%s = %#v, want [] (null is not an empty list)", key, v)
	}
	if len(list) != 0 {
		t.Fatalf("%s = %v, want []", key, list)
	}
}

func requireKeys(t *testing.T, m map[string]interface{}, keys ...string) {
	t.Helper()
	for _, k := range keys {
		if _, ok := m[k]; !ok {
			t.Errorf("required key %q missing from body", k)
		}
	}
}

// SLT text with the characters a careless copy would change: surrounding
// whitespace, a non-ASCII letter, a long string that crosses the 64-byte
// chunk boundary of the hash encoding.
var awkwardSLTs = []string{
	"  Describe the protocol architecture ",
	"Build a basic transaction — with café",
	strings.Repeat("Verify credential hashes across a long line. ", 3),
}

func TestBuildManageModulesBody_RequiredKeysAndEmptyLists(t *testing.T) {
	body, _, err := buildManageModulesBody("teacher-01", "course-abc", []mintModuleSource{
		{Code: "101", Status: "DRAFT", SLTs: []string{"one", "two"}},
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	m := marshalToMap(t, body)

	requireKeys(t, m, "alias", "course_id", "modules_to_add", "modules_to_update", "modules_to_remove")
	if m["alias"] != "teacher-01" || m["course_id"] != "course-abc" {
		t.Errorf("alias/course_id = %v/%v", m["alias"], m["course_id"])
	}
	requireEmptyList(t, m, "modules_to_update")
	requireEmptyList(t, m, "modules_to_remove")

	add := m["modules_to_add"].([]interface{})
	if len(add) != 1 {
		t.Fatalf("modules_to_add has %d entries, want 1", len(add))
	}
	entry := add[0].(map[string]interface{})
	requireKeys(t, entry, "slts", "allowed_student_state_ids", "prereq_credential_hashes")
	requireEmptyList(t, entry, "allowed_student_state_ids")
	requireEmptyList(t, entry, "prereq_credential_hashes")
	// The body names modules by their SLTs only; module_code is not a field.
	if _, ok := entry["module_code"]; ok {
		t.Error("modules_to_add entry carries module_code, which MintModuleV2 does not have")
	}
}

func TestBuildManageModulesBody_SLTTextMatchesRecordByteForByte(t *testing.T) {
	stored := cardano.ComputeSltHash(awkwardSLTs)
	body, summaries, err := buildManageModulesBody("teacher-01", "course-abc", []mintModuleSource{
		{Code: "101", Status: "APPROVED", StoredHash: stored, SLTs: awkwardSLTs},
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	// Through the wire encoding, not just the struct.
	raw, _ := json.Marshal(body)
	var decoded manageModulesBody
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	got := decoded.ModulesToAdd[0].SLTs
	if len(got) != len(awkwardSLTs) {
		t.Fatalf("body has %d SLTs, record has %d", len(got), len(awkwardSLTs))
	}
	for i := range awkwardSLTs {
		if got[i] != awkwardSLTs[i] {
			t.Errorf("slt %d = %q, record holds %q", i, got[i], awkwardSLTs[i])
		}
	}
	if summaries[0].SltHash != stored {
		t.Errorf("summary hash = %s, record hash = %s", summaries[0].SltHash, stored)
	}
	if cardano.ComputeSltHash(got) != stored {
		t.Error("hash of the SLTs in the body differs from the record's slt_hash")
	}
}

func TestBuildManageModulesBody_Refusals(t *testing.T) {
	cases := []struct {
		name string
		src  mintModuleSource
		want string
	}{
		{"on chain", mintModuleSource{Code: "101", Status: "ON_CHAIN", SLTs: []string{"a"}}, "already ON_CHAIN"},
		{"no SLTs", mintModuleSource{Code: "101", Status: "DRAFT"}, "no SLTs"},
		{"hash mismatch", mintModuleSource{Code: "101", Status: "APPROVED", StoredHash: strings.Repeat("0", 64), SLTs: []string{"a"}}, "hash to"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := buildManageModulesBody("t", "c", []mintModuleSource{tc.src})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want one containing %q", err, tc.want)
			}
		})
	}
}

// The teacher list may return SLTs out of slt_index order. The body follows
// slt_index, the order the record's hash was computed in.
func TestModuleSourceFromItem_OrdersBySltIndex(t *testing.T) {
	item := map[string]interface{}{
		"slt_hash": "abc",
		"content": map[string]interface{}{
			"course_module_code": "101",
			"module_status":      "DRAFT",
			"slts": []interface{}{
				map[string]interface{}{"slt_index": float64(2), "slt_text": "second"},
				map[string]interface{}{"slt_index": float64(1), "slt_text": " first "},
			},
		},
	}
	src, ok := moduleSourceFromItem(item)
	if !ok {
		t.Fatal("item not read")
	}
	if len(src.SLTs) != 2 || src.SLTs[0] != " first " || src.SLTs[1] != "second" {
		t.Errorf("SLTs = %q, want [\" first \" \"second\"]", src.SLTs)
	}
	if src.StoredHash != "abc" || src.Status != "DRAFT" || src.Code != "101" {
		t.Errorf("src = %+v", src)
	}
}

func TestBuildManageTasksBody_RequiredKeysAndEmptyLists(t *testing.T) {
	body, summaries, err := buildManageTasksBody("manager-01", "proj-1", "state-1", []mintTaskSource{
		{Index: 1, Status: "DRAFT", Title: "Fix login bug", ExpirationPosix: 1780000000000, LovelaceAmount: 5000000},
	}, nil)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	m := marshalToMap(t, body)

	requireKeys(t, m, "alias", "project_id", "contributor_state_id", "tasks_to_add", "tasks_to_remove", "deposit_value")
	if m["alias"] != "manager-01" || m["project_id"] != "proj-1" || m["contributor_state_id"] != "state-1" {
		t.Errorf("alias/project_id/contributor_state_id = %v/%v/%v", m["alias"], m["project_id"], m["contributor_state_id"])
	}
	requireEmptyList(t, m, "tasks_to_remove")
	requireEmptyList(t, m, "deposit_value")

	add := m["tasks_to_add"].([]interface{})
	if len(add) != 1 {
		t.Fatalf("tasks_to_add has %d entries, want 1", len(add))
	}
	task := add[0].(map[string]interface{})
	requireKeys(t, task, "project_content", "expiration_posix", "lovelace_amount", "native_assets")
	requireEmptyList(t, task, "native_assets")
	if task["project_content"] != "Fix login bug" {
		t.Errorf("project_content = %v", task["project_content"])
	}
	if task["expiration_posix"] != float64(1780000000000) || task["lovelace_amount"] != float64(5000000) {
		t.Errorf("expiration_posix/lovelace_amount = %v/%v", task["expiration_posix"], task["lovelace_amount"])
	}

	wantHash, _ := cardano.ComputeTaskHash(cardano.TaskData{ProjectContent: "Fix login bug", ExpirationTime: 1780000000000, LovelaceAmount: 5000000})
	if summaries[0].TaskHash != wantHash {
		t.Errorf("task hash = %s, want %s", summaries[0].TaskHash, wantHash)
	}
}

func TestBuildManageTasksBody_AssetsAndDeposit(t *testing.T) {
	policy := strings.Repeat("ab", 28)
	deposit, err := buildDepositValue(7000000, []TaskToken{{PolicyID: policy, AssetName: "5850", Quantity: "50"}})
	if err != nil {
		t.Fatalf("deposit: %v", err)
	}
	body, _, err := buildManageTasksBody("m", "p", "s", []mintTaskSource{{
		Index: 2, Status: "DRAFT", Title: "Earn XP", ExpirationPosix: 1780000000000, LovelaceAmount: 2000000,
		Assets: []mintTaskAsset{{PolicyID: policy, AssetName: "5850", Quantity: 50}},
	}}, deposit)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	raw, _ := json.Marshal(body)
	s := string(raw)
	for _, want := range []string{
		`"native_assets":[["` + policy + `.5850",50]]`,
		`"deposit_value":[["lovelace",7000000],["` + policy + `.5850",50]]`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("body %s\nmissing %s", s, want)
		}
	}
}

func TestBuildManageTasksBody_Refusals(t *testing.T) {
	ok := mintTaskSource{Index: 1, Status: "DRAFT", Title: "t", ExpirationPosix: 1780000000000, LovelaceAmount: 1}
	cases := []struct {
		name  string
		mut   func(*mintTaskSource)
		state string
		want  string
	}{
		{"not draft", func(s *mintTaskSource) { s.Status = "ON_CHAIN" }, "s", "only DRAFT"},
		{"no title", func(s *mintTaskSource) { s.Title = "" }, "s", "no title"},
		{"long title", func(s *mintTaskSource) { s.Title = strings.Repeat("x", 141) }, "s", "140"},
		{"no expiration", func(s *mintTaskSource) { s.ExpirationPosix = 0 }, "s", "no expiration"},
		{"no state id", func(s *mintTaskSource) {}, "", "contributor_state_id"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := ok
			tc.mut(&src)
			_, _, err := buildManageTasksBody("m", "p", tc.state, []mintTaskSource{src}, nil)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want one containing %q", err, tc.want)
			}
		})
	}
}

func TestTaskSourceFromItem_ReadsMergedTaskListItem(t *testing.T) {
	item := map[string]interface{}{
		"task_index":       float64(3),
		"task_status":      "DRAFT",
		"lovelace_amount":  float64(5000000),
		"expiration_posix": float64(1780000000), // seconds: normalized to ms
		"assets": []interface{}{
			map[string]interface{}{"policy_id": "pol", "name": "XP", "amount": "50"},
		},
		"content": map[string]interface{}{"title": "Write docs"},
	}
	src, ok := taskSourceFromItem(item)
	if !ok {
		t.Fatal("item not read")
	}
	if src.Index != 3 || src.Status != "DRAFT" || src.Title != "Write docs" || src.LovelaceAmount != 5000000 {
		t.Errorf("src = %+v", src)
	}
	if src.ExpirationPosix != 1780000000000 {
		t.Errorf("expiration = %d, want seconds normalized to ms", src.ExpirationPosix)
	}
	if len(src.Assets) != 1 || src.Assets[0].AssetName != "5850" || src.Assets[0].Quantity != 50 {
		t.Errorf("assets = %+v, want XP hex-encoded as 5850 with quantity 50", src.Assets)
	}
}

func TestTaskSourceFromItem_RejectsOutOfRangeIndex(t *testing.T) {
	for _, raw := range []interface{}{"-1", "2147483648", "9223372036854775807", float64(-5)} {
		if _, ok := taskSourceFromItem(map[string]interface{}{"task_index": raw}); ok {
			t.Errorf("task_index %v read as valid, want rejected", raw)
		}
	}
}

// --- Command level: the body that reaches the gateway's build endpoint. ---

// manageStub serves the reads each command makes and records the first
// /tx/ build request, then fails it so the lifecycle stops before signing.
type manageStub struct {
	mu        sync.Mutex
	reads     map[string]string // path suffix -> response body
	buildPath string
	buildBody map[string]interface{}
	buildRaw  string
	built     bool
}

func (s *manageStub) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if strings.Contains(r.URL.Path, "/tx/") {
			if !s.built {
				s.built = true
				s.buildPath = r.URL.Path
				raw, _ := io.ReadAll(r.Body)
				s.buildRaw = string(raw)
				_ = json.Unmarshal(raw, &s.buildBody)
			}
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"message":"stop here"}`))
			return
		}
		for suffix, resp := range s.reads {
			if strings.HasSuffix(r.URL.Path, suffix) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(resp))
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"no stub"}`))
	}
}

func teacherModulesList(t *testing.T, slts []string, storedHash string) string {
	t.Helper()
	items := make([]interface{}, len(slts))
	for i, s := range slts {
		items[i] = map[string]interface{}{"slt_index": i + 1, "slt_text": s}
	}
	resp := map[string]interface{}{"data": []interface{}{
		map[string]interface{}{
			"slt_hash": storedHash,
			"content": map[string]interface{}{
				"course_module_code": "101",
				"module_status":      "APPROVED",
				"slts":               items,
			},
		},
	}}
	raw, _ := json.Marshal(resp)
	return string(raw)
}

func TestCourseTeacherMintModules_SendsBodyBuiltFromRecord(t *testing.T) {
	stub := &manageStub{reads: map[string]string{
		"/course/teacher/course-modules/list": teacherModulesList(t, awkwardSLTs, cardano.ComputeSltHash(awkwardSLTs)),
	}}
	srv := httptest.NewServer(stub.handler())
	defer srv.Close()

	bin := buildTestBinary(t)
	runCLIWithJWT(t, bin, srv.URL, "test-jwt", nil,
		"course", "teacher", "mint-modules",
		"--course-id", "course-abc", "--alias", "teacher-01", "--module-code", "101",
		"--skey", "/nonexistent/payment.skey", "--submit-url", srv.URL)

	if !stub.built {
		t.Fatal("command never called the build endpoint")
	}
	if stub.buildPath != "/api"+modulesManageEndpoint {
		t.Errorf("build path = %q, want %q", stub.buildPath, "/api"+modulesManageEndpoint)
	}
	requireKeys(t, stub.buildBody, "alias", "course_id", "modules_to_add", "modules_to_update", "modules_to_remove")
	requireEmptyList(t, stub.buildBody, "modules_to_update")
	requireEmptyList(t, stub.buildBody, "modules_to_remove")

	var wire manageModulesBody
	if err := json.Unmarshal([]byte(stub.buildRaw), &wire); err != nil {
		t.Fatalf("decode build body: %v", err)
	}
	got := wire.ModulesToAdd[0].SLTs
	for i := range awkwardSLTs {
		if i >= len(got) || got[i] != awkwardSLTs[i] {
			t.Fatalf("SLTs on the wire = %q, record holds %q", got, awkwardSLTs)
		}
	}
}

func TestCourseTeacherMintModules_DryRunBuildsNothing(t *testing.T) {
	stub := &manageStub{reads: map[string]string{
		"/course/teacher/course-modules/list": teacherModulesList(t, []string{"a", "b"}, ""),
	}}
	srv := httptest.NewServer(stub.handler())
	defer srv.Close()

	bin := buildTestBinary(t)
	stdout, stderr, code := runCLIWithJWT(t, bin, srv.URL, "test-jwt", nil,
		"course", "teacher", "mint-modules",
		"--course-id", "course-abc", "--alias", "teacher-01", "--module-code", "101",
		"--dry-run", "--output", "json")

	if code != 0 {
		t.Fatalf("exit %d, stderr %s", code, stderr)
	}
	if stub.built {
		t.Fatal("--dry-run called the build endpoint")
	}
	var preview ManageBodyPreview
	if err := json.Unmarshal([]byte(stdout), &preview); err != nil {
		t.Fatalf("stdout is not a preview envelope: %v\n%s", err, stdout)
	}
	if !preview.DryRun || preview.TxType != "modules_manage" || len(preview.Modules) != 1 || preview.Modules[0].SltHash != cardano.ComputeSltHash([]string{"a", "b"}) {
		t.Errorf("preview = %+v", preview)
	}
}

func TestCourseTeacherMintModules_UnknownModuleIsNotFound(t *testing.T) {
	stub := &manageStub{reads: map[string]string{
		"/course/teacher/course-modules/list": teacherModulesList(t, []string{"a"}, ""),
	}}
	srv := httptest.NewServer(stub.handler())
	defer srv.Close()

	bin := buildTestBinary(t)
	_, stderr, code := runCLIWithJWT(t, bin, srv.URL, "test-jwt", nil,
		"course", "teacher", "mint-modules",
		"--course-id", "course-abc", "--alias", "teacher-01", "--module-code", "999", "--dry-run")
	if code == 0 || stub.built {
		t.Fatalf("exit %d, built %v; want a failure before any build", code, stub.built)
	}
	if !strings.Contains(stderr, "999") {
		t.Errorf("stderr does not name the missing module: %s", stderr)
	}
}

const managerProjectsList = `{"data":[{"project_id":"proj-1","contributor_state_id":"state-1","content":{"title":"Demo"}}]}`

func TestProjectTaskMint_SendsBodyBuiltFromDraft(t *testing.T) {
	stub := &manageStub{reads: map[string]string{
		"/project/manager/projects/list": managerProjectsList,
		"/project/manager/tasks/list": `{"data":[
			{"task_index":1,"task_status":"DRAFT","lovelace_amount":5000000,"expiration_posix":1780000000000,"content":{"title":"Fix login bug"}},
			{"task_index":2,"task_status":"ON_CHAIN","lovelace_amount":1,"expiration_posix":1780000000000,"content":{"title":"Other"}}
		]}`,
	}}
	srv := httptest.NewServer(stub.handler())
	defer srv.Close()

	bin := buildTestBinary(t)
	runCLIWithJWT(t, bin, srv.URL, "test-jwt", nil,
		"project", "task", "mint", "proj-1", "--alias", "manager-01", "--index", "1",
		"--skey", "/nonexistent/payment.skey", "--submit-url", srv.URL)

	if !stub.built {
		t.Fatal("command never called the build endpoint")
	}
	if stub.buildPath != "/api"+tasksManageEndpoint {
		t.Errorf("build path = %q, want %q", stub.buildPath, "/api"+tasksManageEndpoint)
	}
	requireKeys(t, stub.buildBody, "alias", "project_id", "contributor_state_id", "tasks_to_add", "tasks_to_remove", "deposit_value")
	requireEmptyList(t, stub.buildBody, "tasks_to_remove")
	requireEmptyList(t, stub.buildBody, "deposit_value")
	if stub.buildBody["contributor_state_id"] != "state-1" {
		t.Errorf("contributor_state_id = %v, want state-1 from the manager projects list", stub.buildBody["contributor_state_id"])
	}
	want := `"tasks_to_add":[{"project_content":"Fix login bug","expiration_posix":1780000000000,"lovelace_amount":5000000,"native_assets":[]}]`
	if !strings.Contains(stub.buildRaw, want) {
		t.Errorf("build body %s\nmissing %s", stub.buildRaw, want)
	}
}

func TestProjectTaskMint_RefusesNonDraft(t *testing.T) {
	stub := &manageStub{reads: map[string]string{
		"/project/manager/projects/list": managerProjectsList,
		"/project/manager/tasks/list":    `{"data":[{"task_index":2,"task_status":"ON_CHAIN","lovelace_amount":1,"expiration_posix":1780000000000,"content":{"title":"Other"}}]}`,
	}}
	srv := httptest.NewServer(stub.handler())
	defer srv.Close()

	bin := buildTestBinary(t)
	_, stderr, code := runCLIWithJWT(t, bin, srv.URL, "test-jwt", nil,
		"project", "task", "mint", "proj-1", "--alias", "manager-01", "--index", "2", "--dry-run")
	if code == 0 || stub.built {
		t.Fatalf("exit %d, built %v; want a refusal before any build", code, stub.built)
	}
	if !strings.Contains(stderr, "only DRAFT") {
		t.Errorf("stderr = %s", stderr)
	}
}
