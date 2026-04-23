package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Andamio-Platform/andamio-cli/internal/apierr"
	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/Andamio-Platform/andamio-cli/internal/config"
)

func TestIsModuleAlreadyExistsError(t *testing.T) {
	// Three gates: errors.As(*apierr.ConflictError) + "already exists" + "course_module_code".
	// The type gate blocks non-409 errors; body checks narrow WHICH 409.
	conflict := func(body string) error { return &apierr.ConflictError{Message: body} }

	tests := []struct {
		name string
		err  error
		want bool
	}{
		// Happy paths — type + stem + field all satisfied.
		{"ConflictError with 'already exists' and course_module_code", conflict("API error 409: course_module_code already exists in this course"), true},
		{"ConflictError case-insensitive (mixed case body)", conflict("Course_Module_Code Already Exists"), true},

		// Stem gate negatives — type passes, field passes, but no "already exists".
		{"ConflictError mentioning course_module_code but no 'already exists' stem (validation 409)", conflict("course_module_code is invalid"), false},
		{"ConflictError 'course_module_code must be numeric' (different stem)", conflict("course_module_code must be numeric"), false},

		// Field gate negatives — type passes, stem passes, but no course_module_code.
		{"ConflictError 'module already exists' without course_module_code token", conflict("API error 409: module already exists"), false},
		{"ConflictError 'asset module already exists' adjacent wording", conflict("API error 409: asset module already exists"), false},
		{"ConflictError 'teacher already exists' (different resource)", conflict("API error 409: teacher already exists"), false},

		// Type gate negatives — not a ConflictError, regardless of body.
		{"nil", nil, false},
		{"plain errors.New unrelated", errors.New("boom"), false},
		{"plain errors.New whose body contains both tokens (non-typed)", errors.New("course_module_code already exists"), false},
		{"proxied 5xx body mentioning tokens (*ServerError, not *ConflictError)", &apierr.ServerError{Status: 500, Message: "API error 500: internal error: course_module_code already exists somewhere"}, false},
		{"NotFoundError (wrong type, 404)", &apierr.NotFoundError{Message: "course_module_code already exists"}, false},
		{"AuthError (wrong type, 401/403)", &apierr.AuthError{Message: "course_module_code already exists"}, false},

		// Wrap-chain — errors.As walks through fmt.Errorf(%w).
		{"wrapped ConflictError (via fmt.Errorf %w)", fmt.Errorf("register failed: %w", conflict("API error 409: course_module_code already exists")), true},
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
			got, err := lookupTeacherModule(context.Background(), c, "course-x", tt.moduleCode)

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

// TestRegisterOrRecoverModule_UpdateStatusPayload asserts the update-status call
// on the DRAFT-advance branch carries the correct payload. TestRegisterOrRecoverModule
// below covers envelope shape; this test covers the wire contract of the second POST.
func TestRegisterOrRecoverModule_UpdateStatusPayload(t *testing.T) {
	type call struct {
		path string
		body map[string]interface{}
	}
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
					map[string]interface{}{"content": map[string]interface{}{
						"course_module_code": "101",
						"slt_hash":           "h",
						"module_status":      "DRAFT",
					}},
				},
			})
		case "/api/v2/course/teacher/course-module/update-status":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		}
	}))
	defer srv.Close()

	c := client.New(&config.Config{BaseURL: srv.URL})
	if _, _, err := registerOrRecoverModule(context.Background(), c, "course-x", "101", "h", true); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var updateCall *call
	for i := range calls {
		if calls[i].path == "/api/v2/course/teacher/course-module/update-status" {
			updateCall = &calls[i]
		}
	}
	if updateCall == nil {
		t.Fatal("expected update-status call; got none")
	}
	if updateCall.body["status"] != "APPROVED" {
		t.Errorf("update-status status = %v, want APPROVED", updateCall.body["status"])
	}
	if updateCall.body["slt_hash"] != "h" {
		t.Errorf("update-status slt_hash = %v, want %q", updateCall.body["slt_hash"], "h")
	}
	if updateCall.body["course_module_code"] != "101" {
		t.Errorf("update-status course_module_code = %v, want %q", updateCall.body["course_module_code"], "101")
	}
}

// strPtr is a file-local helper for building *string literals in tests — Go
// forbids taking the address of an unnamed string constant, so &"DRAFT" is a
// compile error. Used by wantAdvancedFrom table values and by a few gateway-
// state helper fixtures.
func strPtr(s string) *string { return &s }

// TestRegisterOrRecoverModule drives the envelope-producing inner function end-to-end
// against an httptest gateway and asserts the exact envelope shape for every success
// branch. The cobra handler is not exercised (config.Load requires filesystem state),
// but every observable contract of the envelope — key presence, action/status values,
// advanced_from nullability, response nesting — is locked in here.
func TestRegisterOrRecoverModule(t *testing.T) {
	tests := []struct {
		name               string
		suppliedHash       string
		registerStatus     int // HTTP status for the register POST; 0 defaults to OK
		registerBody       string
		registerResp       map[string]interface{}
		listStatus         int // HTTP status for the list POST; 0 defaults to OK
		listResponse       map[string]interface{}
		updateStatusStatus int // HTTP status for the update-status POST; 0 defaults to OK
		updateStatusResp   map[string]interface{} // body the gateway returns on update-status; defaults to {"ok": true}
		wantAction         string
		wantStatus         string
		wantSltHash        string
		wantAdvancedFrom   *string
		wantResponseNil    bool
		wantSuccessMsg     string
		wantErrSubstr      string
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
			wantAdvancedFrom: strPtr("DRAFT"),
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
			wantAdvancedFrom: strPtr("DRAFT"),
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
		{
			// Covers the non-conflict error path in registerOrRecoverModule. Guards the
			// "failed to register module:" wrapping so a future refactor that drops the
			// wrap or misroutes non-conflict errors into the recovery branch fails here.
			name:           "non-conflict register error surfaces wrapped",
			suppliedHash:   "abc",
			registerStatus: http.StatusInternalServerError,
			registerBody:   "backend is down",
			wantErrSubstr:  "failed to register module",
		},
		{
			// Covers the update-status failure path inside the DRAFT advance branch.
			// Guards the "advancing to APPROVED failed:" wrapping — a regression that
			// returns the underlying error bare, or silently swallows it, fails here.
			name:           "conflict + DRAFT + matching hash + update-status fails",
			suppliedHash:   "abc",
			registerStatus: http.StatusConflict,
			listResponse: map[string]interface{}{
				"data": []interface{}{
					map[string]interface{}{"content": map[string]interface{}{
						"course_module_code": "101",
						"slt_hash":           "abc",
						"module_status":      "DRAFT",
					}},
				},
			},
			updateStatusStatus: http.StatusInternalServerError,
			wantErrSubstr:      "advancing to APPROVED failed",
		},
		{
			// Covers the default branch when existing.Status is a value the recovery
			// path doesn't explicitly handle (e.g., a future state like "WITHDRAWN").
			// Guards against someone collapsing the switch and dropping the default.
			name:           "conflict + unexpected status + matching hash errors out",
			suppliedHash:   "abc",
			registerStatus: http.StatusConflict,
			listResponse: map[string]interface{}{
				"data": []interface{}{
					map[string]interface{}{"content": map[string]interface{}{
						"course_module_code": "101",
						"slt_hash":           "abc",
						"module_status":      "WITHDRAWN",
					}},
				},
			},
			wantErrSubstr: "unexpected status",
		},
		{
			// Covers the lookup-failure path after a 409. The compound error wraps the
			// lookup failure AND echoes the original gateway 409 via %v. Guards the
			// user-facing message so a future refactor can't drop either half silently.
			name:           "conflict + list lookup fails surfaces compound error",
			suppliedHash:   "abc",
			registerStatus: http.StatusConflict,
			listStatus:     http.StatusInternalServerError,
			wantErrSubstr:  "could not locate it for recovery",
		},
		{
			// R3 lockdown (registered branch): gateway register response carries a
			// status field. Envelope Status reflects it, not the "APPROVED" fallback.
			// Proves the lookupStringField pipe in the registered branch is alive and
			// wins over the hardcoded fallback when the gateway populates the field.
			name:           "registered with gateway-provided status reflects it, not fallback",
			suppliedHash:   "abc123",
			registerStatus: http.StatusOK,
			registerResp: map[string]interface{}{
				"module_id": "m-101",
				"status":    "PENDING_VERIFY",
				"slt_hash":  "canonical_hash",
			},
			wantAction:       "registered",
			wantStatus:       "PENDING_VERIFY",
			wantSltHash:      "canonical_hash",
			wantAdvancedFrom: nil,
			wantResponseNil:  false,
			wantSuccessMsg:   "Module 101: registered.",
		},
		{
			// R3 lockdown (advanced branch): update-status response carries a status
			// field. Envelope Status reflects it, not "APPROVED". Today's real
			// gateway usually returns {"ok": true} here (no status field), but when
			// fixtures land or the gateway expands its response, the envelope tracks.
			name:           "advanced with gateway-provided status reflects it, not fallback",
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
			updateStatusResp: map[string]interface{}{
				"status":   "APPROVED_WITH_WARNING",
				"slt_hash": "canonical_hash",
			},
			wantAction:       "advanced",
			wantStatus:       "APPROVED_WITH_WARNING",
			wantSltHash:      "canonical_hash",
			wantAdvancedFrom: strPtr("DRAFT"),
			wantResponseNil:  false,
			wantSuccessMsg:   "Module 101: advanced from DRAFT to APPROVED.",
		},
		{
			// Fallback coverage: gateway responds 200 with explicit empty-string
			// status and slt_hash fields (distinct from missing fields).
			// lookupStringField's `v != ""` guard should still treat these as
			// "not present" and the envelope falls through to hardcoded values.
			// Catches the case where a future gateway populates the keys but
			// leaves the values blank.
			name:           "registered with empty-string gateway fields falls through to defaults",
			suppliedHash:   "abc123",
			registerStatus: http.StatusOK,
			registerResp: map[string]interface{}{
				"module_id": "m-101",
				"status":    "",
				"slt_hash":  "",
			},
			wantAction:       "registered",
			wantStatus:       "APPROVED", // fallback — gateway empty string is not canonical
			wantSltHash:      "abc123",   // fallback — supplied hash
			wantAdvancedFrom: nil,
			wantResponseNil:  false,
			wantSuccessMsg:   "Module 101: registered.",
		},
		{
			// Whitespace-only fields are also treated as "not present" by the
			// trimmed lookupStringField guard. A buggy or misconfigured gateway
			// returning "status": " " must not leak a single-space value into
			// downstream equality checks against canonical status strings.
			name:           "registered with whitespace-only gateway fields falls through to defaults",
			suppliedHash:   "abc123",
			registerStatus: http.StatusOK,
			registerResp: map[string]interface{}{
				"module_id": "m-101",
				"status":    "   ",
				"slt_hash":  "\t\n",
			},
			wantAction:       "registered",
			wantStatus:       "APPROVED",
			wantSltHash:      "abc123",
			wantAdvancedFrom: nil,
			wantResponseNil:  false,
			wantSuccessMsg:   "Module 101: registered.",
		},
		{
			// Surrounding whitespace on an otherwise-valid value is trimmed to
			// match the canonical form. Prevents " APPROVED " from silently
			// differing from "APPROVED" in downstream comparisons.
			name:           "registered with surrounding-whitespace status gets trimmed",
			suppliedHash:   "abc123",
			registerStatus: http.StatusOK,
			registerResp: map[string]interface{}{
				"module_id": "m-101",
				"status":    "  PENDING_VERIFY  ",
			},
			wantAction:       "registered",
			wantStatus:       "PENDING_VERIFY",
			wantSltHash:      "abc123", // no gateway slt_hash, falls back
			wantAdvancedFrom: nil,
			wantResponseNil:  false,
			wantSuccessMsg:   "Module 101: registered.",
		},
		{
			// Symmetric coverage for the advanced branch: update-status endpoint
			// returns explicit empty-string fields. Mirrors the registered case
			// above. Confirms both branches handle the empty-string edge case
			// identically via the same lookupStringField guard.
			name:           "advanced with empty-string update-status fields falls through to defaults",
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
			updateStatusResp: map[string]interface{}{
				"status":   "",
				"slt_hash": "",
			},
			wantAction:       "advanced",
			wantStatus:       "APPROVED", // fallback
			wantSltHash:      "abc123",   // fallback — supplied
			wantAdvancedFrom: strPtr("DRAFT"),
			wantResponseNil:  false,
			wantSuccessMsg:   "Module 101: advanced from DRAFT to APPROVED.",
		},
		{
			// R4 lockdown (already_registered branch): this is the ONLY branch that
			// guarantees canonical SltHash today. User supplies ABC123 uppercase; DB
			// stores abc123 lowercase; case-insensitive compare matches; envelope
			// returns the stored lowercase, not the supplied uppercase. Catches P2 #8
			// from PR #63 review directly.
			name:           "already_registered returns canonical slt_hash, not supplied casing",
			suppliedHash:   "ABC123",
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/v2/course/teacher/course-module/register":
					switch {
					case tt.registerStatus == 0 || tt.registerStatus == http.StatusOK:
						_ = json.NewEncoder(w).Encode(tt.registerResp)
					case tt.registerStatus == http.StatusConflict:
						http.Error(w, "course_module_code already exists in this course", tt.registerStatus)
					default:
						body := tt.registerBody
						if body == "" {
							body = "error"
						}
						http.Error(w, body, tt.registerStatus)
					}
				case "/api/v2/course/teacher/course-modules/list":
					if tt.listStatus != 0 && tt.listStatus != http.StatusOK {
						http.Error(w, "list failed", tt.listStatus)
						return
					}
					_ = json.NewEncoder(w).Encode(tt.listResponse)
				case "/api/v2/course/teacher/course-module/update-status":
					if tt.updateStatusStatus != 0 && tt.updateStatusStatus != http.StatusOK {
						http.Error(w, "update-status failed", tt.updateStatusStatus)
						return
					}
					body := tt.updateStatusResp
					if body == nil {
						body = map[string]interface{}{"ok": true}
					}
					_ = json.NewEncoder(w).Encode(body)
				default:
					t.Errorf("unexpected path: %s", r.URL.Path)
				}
			}))
			defer srv.Close()

			c := client.New(&config.Config{BaseURL: srv.URL})
			envelope, successMsg, err := registerOrRecoverModule(context.Background(), c, "course-x", "101", tt.suppliedHash, true)

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

			if envelope.Action != tt.wantAction {
				t.Errorf("Action = %v, want %v", envelope.Action, tt.wantAction)
			}
			if envelope.Status != tt.wantStatus {
				t.Errorf("Status = %v, want %v", envelope.Status, tt.wantStatus)
			}
			if envelope.SltHash != tt.wantSltHash {
				t.Errorf("SltHash = %v, want %v", envelope.SltHash, tt.wantSltHash)
			}
			// Pointer-value compare: nil-check both sides, then dereference when
			// both non-nil. Pointer identity is wrong here (each test builds its
			// own *string via strPtr).
			switch {
			case tt.wantAdvancedFrom == nil && envelope.AdvancedFrom != nil:
				t.Errorf("AdvancedFrom = %q, want nil", *envelope.AdvancedFrom)
			case tt.wantAdvancedFrom != nil && envelope.AdvancedFrom == nil:
				t.Errorf("AdvancedFrom = nil, want %q", *tt.wantAdvancedFrom)
			case tt.wantAdvancedFrom != nil && envelope.AdvancedFrom != nil && *envelope.AdvancedFrom != *tt.wantAdvancedFrom:
				t.Errorf("AdvancedFrom = %q, want %q", *envelope.AdvancedFrom, *tt.wantAdvancedFrom)
			}
			if tt.wantResponseNil && envelope.Response != nil {
				t.Errorf("Response = %v, want nil", envelope.Response)
			}
			if !tt.wantResponseNil && envelope.Response == nil {
				t.Errorf("Response is nil, want non-nil gateway response")
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

// TestRegisterOrRecover_RetriesTransientListFailure pins the Unit 6 contract
// from issue #65: a transient 5xx on the teacher-modules-list recovery call
// must NOT be fatal — the lookup retries and the recovery proceeds normally
// once the gateway responds. Without PostWithRetry in lookupTeacherModule,
// this test fails with "could not locate it for recovery".
func TestRegisterOrRecover_RetriesTransientListFailure(t *testing.T) {
	var registerCalls, listCalls, updateCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/course/teacher/course-module/register":
			registerCalls++
			http.Error(w, "course_module_code already exists in this course", http.StatusConflict)
		case "/api/v2/course/teacher/course-modules/list":
			listCalls++
			// First list call returns 502; second returns the module in DRAFT.
			if listCalls == 1 {
				http.Error(w, "bad gateway", http.StatusBadGateway)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"data": []interface{}{
					map[string]interface{}{"content": map[string]interface{}{
						"course_module_code": "101",
						"slt_hash":           "abc",
						"module_status":      "DRAFT",
					}},
				},
			})
		case "/api/v2/course/teacher/course-module/update-status":
			updateCalls++
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	c := client.New(&config.Config{BaseURL: srv.URL})
	envelope, _, err := registerOrRecoverModule(context.Background(), c, "course-x", "101", "abc", true)
	if err != nil {
		t.Fatalf("expected recovery to succeed despite transient 502, got: %v", err)
	}
	if envelope.Action != "advanced" {
		t.Errorf("Action = %v, want 'advanced'", envelope.Action)
	}
	if listCalls < 2 {
		t.Errorf("expected list call to be retried at least once; got %d calls", listCalls)
	}
	if registerCalls != 1 {
		t.Errorf("register should not be retried (409 non-retryable); got %d calls", registerCalls)
	}
	if updateCalls != 1 {
		t.Errorf("update-status should be called exactly once after successful list; got %d", updateCalls)
	}
}

// TestRegisterOrRecover_OnRetryCallbackFires asserts that when the retry
// path is taken during recovery, the Client.SetOnRetry callback is invoked.
// This pins the CHANGELOG promise that "retrying..." messages appear on
// stderr (the command-layer callback logs the attempt; this test verifies
// the wiring).
func TestRegisterOrRecover_OnRetryCallbackFires(t *testing.T) {
	var listCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/course/teacher/course-module/register":
			http.Error(w, "course_module_code already exists in this course", http.StatusConflict)
		case "/api/v2/course/teacher/course-modules/list":
			listCalls++
			if listCalls == 1 {
				http.Error(w, "bad gateway", http.StatusBadGateway)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"data": []interface{}{
					map[string]interface{}{"content": map[string]interface{}{
						"course_module_code": "101",
						"slt_hash":           "abc",
						"module_status":      "DRAFT",
					}},
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
	var callbacks int
	c.SetOnRetry(func(attempt int, wait time.Duration, err error) {
		callbacks++
		if err == nil {
			t.Errorf("OnRetry received nil err")
		}
		if attempt < 2 {
			t.Errorf("attempt = %d, want >=2 (first retry)", attempt)
		}
	})

	if _, _, err := registerOrRecoverModule(context.Background(), c, "course-x", "101", "abc", true); err != nil {
		t.Fatalf("recovery should succeed, got: %v", err)
	}
	if callbacks != 1 {
		t.Errorf("expected 1 retry callback invocation, got %d", callbacks)
	}
}

// TestRegisterOrRecover_WrapChainPreservesServerError_OnListExhaustion asserts that
// when the list call exhausts retries, the final error still unwraps to
// *apierr.ServerError via errors.As through two fmt.Errorf(%w) wrap layers.
// This guarantees future callers (exit-code mapping, upstream wrappers) can
// branch on 5xx.
func TestRegisterOrRecover_WrapChainPreservesServerError_OnListExhaustion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/course/teacher/course-module/register":
			http.Error(w, "course_module_code already exists in this course", http.StatusConflict)
		case "/api/v2/course/teacher/course-modules/list":
			http.Error(w, "bad gateway", http.StatusBadGateway)
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	c := client.New(&config.Config{BaseURL: srv.URL})
	_, _, err := registerOrRecoverModule(context.Background(), c, "course-x", "101", "abc", true)
	if err == nil {
		t.Fatal("expected error after list exhaustion")
	}
	var serverErr *apierr.ServerError
	if !errors.As(err, &serverErr) {
		t.Fatalf("errors.As should unwrap *ServerError through double fmt.Errorf(%%w); got %T: %v", err, err)
	}
	if serverErr.Status != 502 {
		t.Errorf("ServerError.Status = %d, want 502", serverErr.Status)
	}
	if !strings.Contains(err.Error(), "could not locate it for recovery") {
		t.Errorf("outer error message should preserve recovery context; got %q", err.Error())
	}
}
