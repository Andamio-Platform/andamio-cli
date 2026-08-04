package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// course owner teachers moved from a DB-proxy POST to the transaction path in
// #140. The route it used to call, /api/v2/course/owner/teachers/update, was
// removed by andamio-api#689 and does not exist in API 2.5 — the only API
// version CLI 1.0 supports. These tests pin the three things that would let
// that regress silently: the endpoint, the tx type, and the wire field names.
//
// The gateway's request struct is ManageTeachersTxRequest:
//
//	alias, course_id, teachers_to_add, teachers_to_remove
//
// The CLI's own surface keeps --add/--remove/--alias. The mapping between the
// two lives in exactly one place, so these assertions are the guard on it.

// captureTxBuild stands in for the gateway's tx-build endpoint. It records the
// first path and body it is asked for, then fails the build so the lifecycle
// stops before it needs a real .skey, a submit URL, or a chain. Everything
// under test happens before that point.
type captureTxBuild struct {
	mu   sync.Mutex
	path string
	body map[string]interface{}
	seen bool
}

func (c *captureTxBuild) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c.mu.Lock()
		defer c.mu.Unlock()
		if !c.seen && strings.Contains(r.URL.Path, "/tx/") {
			c.seen = true
			c.path = r.URL.Path
			raw, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(raw, &c.body)
		}
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"stop here"}`))
	}
}

func runTeachersCmd(t *testing.T, extraArgs ...string) *captureTxBuild {
	t.Helper()

	cap := &captureTxBuild{}
	srv := httptest.NewServer(cap.handler())
	defer srv.Close()

	bin := buildTestBinary(t)
	args := append([]string{
		"course", "owner", "teachers",
		"--course-id", "course-abc",
		"--alias", "owner-01",
		"--skey", "/nonexistent/payment.skey",
		// executeTxLifecycle validates a submit URL before it builds, so the
		// command now needs one configured even to reach the build step.
		"--submit-url", srv.URL,
		"--add", "alice",
		"--add", "bob",
		"--remove", "charlie",
	}, extraArgs...)

	runCLIWithJWT(t, bin, srv.URL, "test-jwt", nil, args...)

	if !cap.seen {
		t.Fatal("command never called a /tx/ endpoint — it is not on the transaction path")
	}
	return cap
}

// The command must target the replacement route. The old one is gone in 2.5.
func TestCourseOwnerTeachers_UsesTxManageEndpoint(t *testing.T) {
	cap := runTeachersCmd(t)

	const want = "/api/v2/tx/course/owner/teachers/manage"
	if cap.path != want {
		t.Errorf("endpoint = %q, want %q", cap.path, want)
	}
	if strings.Contains(cap.path, "/course/owner/teachers/update") {
		t.Error("still calling the route andamio-api#689 removed (#140)")
	}
}

// The gateway's field names differ from the CLI's flag names. A rename on
// either side that isn't mirrored here produces a silently wrong transaction,
// not an error — teachers_to_add absent reads as "add nobody".
func TestCourseOwnerTeachers_SendsGatewayFieldNames(t *testing.T) {
	cap := runTeachersCmd(t)

	if got, ok := cap.body["alias"].(string); !ok || got != "owner-01" {
		t.Errorf("alias = %v, want %q (this is the OWNER, not a teacher)", cap.body["alias"], "owner-01")
	}
	if got, ok := cap.body["course_id"].(string); !ok || got != "course-abc" {
		t.Errorf("course_id = %v, want %q", cap.body["course_id"], "course-abc")
	}

	add, ok := cap.body["teachers_to_add"].([]interface{})
	if !ok {
		t.Fatalf("teachers_to_add missing or not an array: %#v", cap.body["teachers_to_add"])
	}
	if len(add) != 2 || add[0] != "alice" || add[1] != "bob" {
		t.Errorf("teachers_to_add = %v, want [alice bob]", add)
	}

	remove, ok := cap.body["teachers_to_remove"].([]interface{})
	if !ok {
		t.Fatalf("teachers_to_remove missing or not an array: %#v", cap.body["teachers_to_remove"])
	}
	if len(remove) != 1 || remove[0] != "charlie" {
		t.Errorf("teachers_to_remove = %v, want [charlie]", remove)
	}

	// The pre-#140 payload shape must not survive anywhere.
	for _, dead := range []string{"add", "remove"} {
		if _, present := cap.body[dead]; present {
			t.Errorf("payload still carries the old %q key", dead)
		}
	}
}

// Both arrays ride even when one side of the change is empty. An absent key is
// not the same claim as an empty one, and the transaction describes the whole
// requested change.
func TestCourseOwnerTeachers_SendsEmptyArrayNotOmitted(t *testing.T) {
	cap := &captureTxBuild{}
	srv := httptest.NewServer(cap.handler())
	defer srv.Close()

	bin := buildTestBinary(t)
	runCLIWithJWT(t, bin, srv.URL, "test-jwt", nil,
		"course", "owner", "teachers",
		"--course-id", "course-abc",
		"--alias", "owner-01",
		"--skey", "/nonexistent/payment.skey",
		"--submit-url", srv.URL,
		"--add", "alice",
	)

	if !cap.seen {
		t.Fatal("command never called a /tx/ endpoint")
	}
	remove, present := cap.body["teachers_to_remove"]
	if !present {
		t.Fatal("teachers_to_remove omitted; it must be sent as an empty array")
	}
	if arr, ok := remove.([]interface{}); !ok || len(arr) != 0 {
		t.Errorf("teachers_to_remove = %v, want []", remove)
	}
}

// --skey and --alias are required. Without them the command cannot build a
// transaction, and cobra should say so rather than the gateway rejecting a
// half-formed payload.
func TestCourseOwnerTeachers_RequiresSkeyAndAlias(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	bin := buildTestBinary(t)

	cases := map[string][]string{
		"missing skey":  {"course", "owner", "teachers", "--course-id", "c", "--alias", "o", "--add", "alice"},
		"missing alias": {"course", "owner", "teachers", "--course-id", "c", "--skey", "k", "--add", "alice"},
	}

	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			_, stderr, code := runCLIWithJWT(t, bin, srv.URL, "test-jwt", nil, args...)
			if code == 0 {
				t.Errorf("exit 0 with a required flag missing; stderr: %s", stderr)
			}
			if !strings.Contains(stderr, "required") {
				t.Errorf("stderr does not explain the missing flag: %q", stderr)
			}
		})
	}
}
