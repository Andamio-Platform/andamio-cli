package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/Andamio-Platform/andamio-cli/internal/config"
)

func TestIsModuleAlreadyExistsError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"unrelated error", errors.New("boom"), false},
		{"already exists with course_module_code", errors.New("API error 409: course_module_code already exists in this course"), true},
		{"case insensitive", errors.New("Course_Module_Code Already Exists"), true},
		{"already exists with bare 'module' token (no course_module_code) is NOT a course-module conflict", errors.New("API error 409: module already exists"), false},
		{"already exists in proxied 5xx body mentioning 'module' is NOT a course-module conflict", errors.New("API error 500: internal error in module github.com/foo: record already exists"), false},
		{"already exists with adjacent 'asset module' wording", errors.New("API error 409: asset module already exists"), false},
		{"already exists without module context (e.g. duplicate teacher)", errors.New("API error 409: teacher already exists"), false},
		{"different stem (not a duplicate)", errors.New("course_module_code is invalid"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isModuleAlreadyExistsError(tt.err); got != tt.want {
				t.Errorf("isModuleAlreadyExistsError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestNormalizeSltHashFlag(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{"valid hash unchanged", "abc123", "abc123", false},
		{"leading whitespace trimmed", " abc123", "abc123", false},
		{"trailing whitespace trimmed", "abc123 ", "abc123", false},
		{"both sides trimmed", "  abc123  ", "abc123", false},
		{"tabs and newlines trimmed", "\tabc123\n", "abc123", false},
		{"empty rejected", "", "", true},
		{"whitespace-only rejected", "   ", "", true},
		{"tabs-only rejected", "\t\t", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeSltHashFlag(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("normalizeSltHashFlag(%q) = %q, nil; want error", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Errorf("normalizeSltHashFlag(%q) returned unexpected error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("normalizeSltHashFlag(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestMismatchError(t *testing.T) {
	gateway := errors.New("API error 409: course_module_code already exists")
	err := mismatchError("course-x", "101", "stored", "supplied", gateway)
	msg := err.Error()

	for _, want := range []string{"stored", "supplied", "delete-module --course-id course-x --module-code 101"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error missing %q: %s", want, msg)
		}
	}
	if !errors.Is(err, gateway) {
		t.Errorf("wrapped gateway error not unwrappable via errors.Is")
	}
}

func TestLookupStringField(t *testing.T) {
	tests := []struct {
		name  string
		m     map[string]interface{}
		names []string
		want  string
	}{
		{
			name:  "top-level match",
			m:     map[string]interface{}{"slt_hash": "abc"},
			names: []string{"slt_hash"},
			want:  "abc",
		},
		{
			name:  "content-nested match when top-level absent",
			m:     map[string]interface{}{"content": map[string]interface{}{"slt_hash": "def"}},
			names: []string{"slt_hash"},
			want:  "def",
		},
		{
			name:  "top-level wins over content",
			m:     map[string]interface{}{"slt_hash": "top", "content": map[string]interface{}{"slt_hash": "nested"}},
			names: []string{"slt_hash"},
			want:  "top",
		},
		{
			name:  "fallback field name at top level",
			m:     map[string]interface{}{"course_module_slt_hash": "xyz"},
			names: []string{"slt_hash", "course_module_slt_hash"},
			want:  "xyz",
		},
		{
			name:  "fallback field name in content",
			m:     map[string]interface{}{"content": map[string]interface{}{"module_status": "DRAFT"}},
			names: []string{"course_module_status", "module_status", "status"},
			want:  "DRAFT",
		},
		{
			name:  "empty string skipped, falls back to next name",
			m:     map[string]interface{}{"slt_hash": "", "course_module_slt_hash": "fallback"},
			names: []string{"slt_hash", "course_module_slt_hash"},
			want:  "fallback",
		},
		{
			name:  "no match",
			m:     map[string]interface{}{"unrelated": "x"},
			names: []string{"slt_hash"},
			want:  "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := lookupStringField(tt.m, tt.names...); got != tt.want {
				t.Errorf("lookupStringField = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLookupTeacherModule(t *testing.T) {
	tests := []struct {
		name       string
		response   map[string]interface{}
		moduleCode string
		wantHash   string
		wantStatus string
		wantErr    string // substring; empty means no error
	}{
		{
			name: "found, content-nested shape (mirrors course_export.go)",
			response: map[string]interface{}{
				"data": []interface{}{
					map[string]interface{}{
						"content": map[string]interface{}{
							"course_module_code": "101",
							"slt_hash":           "hash101",
							"module_status":      "DRAFT",
						},
					},
					map[string]interface{}{
						"content": map[string]interface{}{
							"course_module_code": "102",
							"slt_hash":           "hash102",
							"module_status":      "ON_CHAIN",
						},
					},
				},
			},
			moduleCode: "102",
			wantHash:   "hash102",
			wantStatus: "ON_CHAIN",
		},
		{
			name: "found, top-level shape",
			response: map[string]interface{}{
				"data": []interface{}{
					map[string]interface{}{
						"course_module_code":   "201",
						"course_module_slt_hash": "h201",
						"course_module_status": "APPROVED",
					},
				},
			},
			moduleCode: "201",
			wantHash:   "h201",
			wantStatus: "APPROVED",
		},
		{
			name: "not found",
			response: map[string]interface{}{
				"data": []interface{}{
					map[string]interface{}{"content": map[string]interface{}{"course_module_code": "999"}},
				},
			},
			moduleCode: "101",
			wantErr:    "not found in teacher list",
		},
		{
			name:       "missing data array",
			response:   map[string]interface{}{},
			moduleCode: "101",
			wantErr:    "missing data array",
		},
		{
			name: "matched module missing slt_hash surfaces response-shape error",
			response: map[string]interface{}{
				"data": []interface{}{
					map[string]interface{}{
						"content": map[string]interface{}{
							"course_module_code": "101",
							"module_status":      "DRAFT",
						},
					},
				},
			},
			moduleCode: "101",
			wantErr:    "slt_hash field is missing",
		},
		{
			name: "matched module missing status surfaces response-shape error",
			response: map[string]interface{}{
				"data": []interface{}{
					map[string]interface{}{
						"content": map[string]interface{}{
							"course_module_code": "101",
							"slt_hash":           "abc",
						},
					},
				},
			},
			moduleCode: "101",
			wantErr:    "status field is missing",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/v2/course/teacher/course-modules/list" {
					t.Errorf("unexpected path: %s", r.URL.Path)
				}
				_ = json.NewEncoder(w).Encode(tt.response)
			}))
			defer srv.Close()

			c := client.New(&config.Config{BaseURL: srv.URL})
			got, err := lookupTeacherModule(c, "course-x", tt.moduleCode)

			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want substring %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if got.SltHash != tt.wantHash {
				t.Errorf("SltHash = %q, want %q", got.SltHash, tt.wantHash)
			}
			if got.Status != tt.wantStatus {
				t.Errorf("Status = %q, want %q", got.Status, tt.wantStatus)
			}
		})
	}
}

// TestRegisterModuleConflictBranches exercises the conflict-recovery branching of
// runCourseTeacherRegisterModule end-to-end against an httptest gateway. It does not
// invoke the cobra handler directly (which would require config.Load and global
// flag state); instead it composes the same building blocks the handler uses.
func TestRegisterModuleConflictBranches(t *testing.T) {
	type call struct {
		path string
		body map[string]interface{}
	}

	tests := []struct {
		name           string
		suppliedHash   string
		existingHash   string
		existingStatus string
		wantAction     string // empty if expected to error
		wantStatus     string
		wantErr        string // substring
	}{
		{"DRAFT match advances to APPROVED", "h", "h", "DRAFT", "advanced", "APPROVED", ""},
		{"APPROVED match is no-op", "h", "h", "APPROVED", "already_registered", "APPROVED", ""},
		{"PENDING_TX match is no-op", "h", "h", "PENDING_TX", "already_registered", "PENDING_TX", ""},
		{"ON_CHAIN match is no-op", "h", "h", "ON_CHAIN", "already_registered", "ON_CHAIN", ""},
		{"hash mismatch errors with both hashes", "supplied", "stored", "DRAFT", "", "", "supplied"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls []call
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]interface{}
				_ = json.NewDecoder(r.Body).Decode(&body)
				calls = append(calls, call{path: r.URL.Path, body: body})

				switch r.URL.Path {
				case "/api/v2/course/teacher/course-module/register":
					http.Error(w, "course_module_code already exists in this course", http.StatusConflict)
				case "/api/v2/course/teacher/course-modules/list":
					_ = json.NewEncoder(w).Encode(map[string]interface{}{
						"data": []interface{}{
							map[string]interface{}{
								"content": map[string]interface{}{
									"course_module_code": "101",
									"slt_hash":           tt.existingHash,
									"module_status":      tt.existingStatus,
								},
							},
						},
					})
				case "/api/v2/course/teacher/course-module/update-status":
					_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
				default:
					t.Errorf("unexpected path: %s", r.URL.Path)
				}
			}))
			defer srv.Close()

			c := client.New(&config.Config{BaseURL: srv.URL})

			// Mirror handler logic: register, detect conflict, lookup, branch.
			_, regErr := postRegisterModule(c, "course-x", "101", tt.suppliedHash)
			if regErr == nil {
				t.Fatal("expected register to fail")
			}
			if !isModuleAlreadyExistsError(regErr) {
				t.Fatalf("expected conflict, got: %v", regErr)
			}

			existing, lookupErr := lookupTeacherModule(c, "course-x", "101")
			if lookupErr != nil {
				t.Fatalf("lookup failed: %v", lookupErr)
			}

			if existing.SltHash != tt.suppliedHash {
				// Mismatch branch — reproduce the handler's exact error formatting and assert
				// it names both hashes, includes the delete-module hint, and unwraps to the
				// original gateway 409 error so consumers can use errors.Unwrap.
				if tt.wantErr == "" {
					t.Fatalf("expected match, but got mismatch: %s vs %s", existing.SltHash, tt.suppliedHash)
				}
				mismatchErr := mismatchError("course-x", "101", existing.SltHash, tt.suppliedHash, regErr)
				msg := mismatchErr.Error()
				if !strings.Contains(msg, existing.SltHash) {
					t.Errorf("mismatch error missing existing hash %q: %s", existing.SltHash, msg)
				}
				if !strings.Contains(msg, tt.suppliedHash) {
					t.Errorf("mismatch error missing supplied hash %q: %s", tt.suppliedHash, msg)
				}
				if !strings.Contains(msg, "delete-module --course-id course-x --module-code 101") {
					t.Errorf("mismatch error missing delete-module remediation: %s", msg)
				}
				if unwrapped := errors.Unwrap(mismatchErr); unwrapped == nil {
					t.Errorf("mismatch error did not wrap the original gateway error")
				} else if !strings.Contains(unwrapped.Error(), "course_module_code") {
					t.Errorf("unwrapped error did not preserve gateway message: %v", unwrapped)
				}
				return
			}

			// Match branches
			switch existing.Status {
			case "DRAFT":
				if tt.wantAction != "advanced" {
					t.Fatalf("DRAFT branch hit but wantAction=%q", tt.wantAction)
				}
				if _, err := postUpdateModuleStatus(c, "course-x", "101", "APPROVED", tt.suppliedHash); err != nil {
					t.Fatalf("update-status failed: %v", err)
				}
				// Verify update-status was called with slt_hash
				lastCall := calls[len(calls)-1]
				if lastCall.path != "/api/v2/course/teacher/course-module/update-status" {
					t.Errorf("expected update-status call, got %s", lastCall.path)
				}
				if lastCall.body["status"] != "APPROVED" || lastCall.body["slt_hash"] != tt.suppliedHash {
					t.Errorf("update-status payload mismatch: %v", lastCall.body)
				}
			case "APPROVED", "PENDING_TX", "ON_CHAIN":
				if tt.wantAction != "already_registered" {
					t.Fatalf("%s branch hit but wantAction=%q", existing.Status, tt.wantAction)
				}
				if existing.Status != tt.wantStatus {
					t.Errorf("status = %q, want %q", existing.Status, tt.wantStatus)
				}
			}
		})
	}
}

// TestRegisterOrRecoverModule drives the envelope-producing inner function end-to-end
// against an httptest gateway and asserts the exact envelope shape for every success
// branch. The cobra handler is not exercised (config.Load requires filesystem state),
// but every observable contract of the envelope — key presence, action/status values,
// advanced_from nullability, response nesting — is locked in here.
func TestRegisterOrRecoverModule(t *testing.T) {
	tests := []struct {
		name             string
		suppliedHash     string
		registerStatus   int  // HTTP status for the register POST
		registerResp     map[string]interface{}
		listResponse     map[string]interface{}
		wantAction       string
		wantStatus       string
		wantSltHash      string
		wantAdvancedFrom interface{}
		wantResponseNil  bool
		wantSuccessMsg   string
		wantErrSubstr    string
	}{
		{
			name:             "happy path — gateway accepts, action=registered",
			suppliedHash:     "abc123",
			registerStatus:   http.StatusOK,
			registerResp:     map[string]interface{}{"module_id": "m-101"},
			wantAction:       "registered",
			wantStatus:       "APPROVED",
			wantSltHash:      "abc123",
			wantAdvancedFrom: nil,
			wantResponseNil:  false,
			wantSuccessMsg:   "Module 101: registered.",
		},
		{
			name:           "conflict + DRAFT + matching hash, action=advanced",
			suppliedHash:   "abc123",
			registerStatus: http.StatusConflict,
			listResponse: map[string]interface{}{
				"data": []interface{}{
					map[string]interface{}{"content": map[string]interface{}{
						"course_module_code": "101",
						"slt_hash":           "abc123",
						"module_status":      "DRAFT",
					}},
				},
			},
			wantAction:       "advanced",
			wantStatus:       "APPROVED",
			wantSltHash:      "abc123",
			wantAdvancedFrom: "DRAFT",
			wantResponseNil:  false,
			wantSuccessMsg:   "Module 101: advanced from DRAFT to APPROVED.",
		},
		{
			name:           "conflict + APPROVED + matching hash, action=already_registered",
			suppliedHash:   "abc123",
			registerStatus: http.StatusConflict,
			listResponse: map[string]interface{}{
				"data": []interface{}{
					map[string]interface{}{"content": map[string]interface{}{
						"course_module_code": "101",
						"slt_hash":           "abc123",
						"module_status":      "APPROVED",
					}},
				},
			},
			wantAction:       "already_registered",
			wantStatus:       "APPROVED",
			wantSltHash:      "abc123",
			wantAdvancedFrom: nil,
			wantResponseNil:  true,
			wantSuccessMsg:   "Module 101: already registered (status: APPROVED).",
		},
		{
			name:           "conflict + ON_CHAIN + matching hash, action=already_registered",
			suppliedHash:   "abc123",
			registerStatus: http.StatusConflict,
			listResponse: map[string]interface{}{
				"data": []interface{}{
					map[string]interface{}{"content": map[string]interface{}{
						"course_module_code": "101",
						"slt_hash":           "abc123",
						"module_status":      "ON_CHAIN",
					}},
				},
			},
			wantAction:       "already_registered",
			wantStatus:       "ON_CHAIN",
			wantSltHash:      "abc123",
			wantAdvancedFrom: nil,
			wantResponseNil:  true,
			wantSuccessMsg:   "Module 101: already registered (status: ON_CHAIN).",
		},
		{
			name:           "case-insensitive hash match (supplied upper, existing lower) advances cleanly",
			suppliedHash:   "ABC123",
			registerStatus: http.StatusConflict,
			listResponse: map[string]interface{}{
				"data": []interface{}{
					map[string]interface{}{"content": map[string]interface{}{
						"course_module_code": "101",
						"slt_hash":           "abc123",
						"module_status":      "DRAFT",
					}},
				},
			},
			wantAction:       "advanced",
			wantStatus:       "APPROVED",
			wantSltHash:      "ABC123", // supplied value is preserved in envelope
			wantAdvancedFrom: "DRAFT",
			wantResponseNil:  false,
			wantSuccessMsg:   "Module 101: advanced from DRAFT to APPROVED.",
		},
		{
			name:           "hash mismatch routes to mismatchError, no envelope",
			suppliedHash:   "supplied",
			registerStatus: http.StatusConflict,
			listResponse: map[string]interface{}{
				"data": []interface{}{
					map[string]interface{}{"content": map[string]interface{}{
						"course_module_code": "101",
						"slt_hash":           "stored",
						"module_status":      "DRAFT",
					}},
				},
			},
			wantErrSubstr: "stored",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/v2/course/teacher/course-module/register":
					if tt.registerStatus == http.StatusOK {
						_ = json.NewEncoder(w).Encode(tt.registerResp)
					} else {
						http.Error(w, "course_module_code already exists in this course", tt.registerStatus)
					}
				case "/api/v2/course/teacher/course-modules/list":
					_ = json.NewEncoder(w).Encode(tt.listResponse)
				case "/api/v2/course/teacher/course-module/update-status":
					_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
				default:
					t.Errorf("unexpected path: %s", r.URL.Path)
				}
			}))
			defer srv.Close()

			c := client.New(&config.Config{BaseURL: srv.URL})
			envelope, successMsg, err := registerOrRecoverModule(c, "course-x", "101", tt.suppliedHash)

			if tt.wantErrSubstr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErrSubstr) {
					t.Fatalf("err = %v, want substring %q", err, tt.wantErrSubstr)
				}
				if envelope != nil {
					t.Errorf("envelope should be nil on error, got %v", envelope)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if envelope["action"] != tt.wantAction {
				t.Errorf("action = %v, want %v", envelope["action"], tt.wantAction)
			}
			if envelope["status"] != tt.wantStatus {
				t.Errorf("status = %v, want %v", envelope["status"], tt.wantStatus)
			}
			if envelope["slt_hash"] != tt.wantSltHash {
				t.Errorf("slt_hash = %v, want %v", envelope["slt_hash"], tt.wantSltHash)
			}
			if envelope["advanced_from"] != tt.wantAdvancedFrom {
				t.Errorf("advanced_from = %v, want %v", envelope["advanced_from"], tt.wantAdvancedFrom)
			}
			if tt.wantResponseNil && envelope["response"] != nil {
				t.Errorf("response = %v, want nil", envelope["response"])
			}
			if !tt.wantResponseNil && envelope["response"] == nil {
				t.Errorf("response is nil, want non-nil gateway response")
			}
			if successMsg != tt.wantSuccessMsg {
				t.Errorf("successMsg = %q, want %q", successMsg, tt.wantSuccessMsg)
			}

			// Round-trip through JSON to verify advanced_from serializes as JSON null
			// (not the string "null", not absent) and that all five keys survive.
			jsonBytes, err := json.Marshal(envelope)
			if err != nil {
				t.Fatalf("marshal failed: %v", err)
			}
			var parsed map[string]interface{}
			if err := json.Unmarshal(jsonBytes, &parsed); err != nil {
				t.Fatalf("unmarshal failed: %v", err)
			}
			for _, key := range []string{"action", "status", "slt_hash", "advanced_from", "response"} {
				if _, ok := parsed[key]; !ok {
					t.Errorf("envelope missing key %q after JSON round-trip: %s", key, string(jsonBytes))
				}
			}
			if tt.wantAdvancedFrom == nil && parsed["advanced_from"] != nil {
				t.Errorf("advanced_from serialized as %v, want JSON null", parsed["advanced_from"])
			}
		})
	}
}
