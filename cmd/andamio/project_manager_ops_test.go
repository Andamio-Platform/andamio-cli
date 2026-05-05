package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Andamio-Platform/andamio-cli/internal/apierr"
	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/Andamio-Platform/andamio-cli/internal/config"
)

// stubServer wires an httptest.Server with a handler and a client that points
// at it. Returns both so tests can assert on the captured request and close
// the server.
func stubQualifiedServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *client.Client) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c := client.New(&config.Config{BaseURL: srv.URL})
	return srv, c
}

func TestFetchQualifiedContributors_HappyPath(t *testing.T) {
	var gotPath string
	var gotAPIKey, gotAuth string
	_, c := stubQualifiedServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path + "?" + r.URL.RawQuery
		gotAPIKey = r.Header.Get("X-API-Key")
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"projectId":  "P",
			"aliases":    []string{"ada", "alan"},
			"totalCount": 2,
			"truncated":  false,
		})
	})

	resp, err := fetchQualifiedContributors(context.Background(), c, "P")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if want := "/api/v2/project/manager/contributors/get-qualified?project_id=P"; gotPath != want {
		t.Errorf("path = %q, want %q", gotPath, want)
	}
	if resp.ProjectID != "P" {
		t.Errorf("ProjectID = %q, want %q", resp.ProjectID, "P")
	}
	if got, want := resp.Aliases, []string{"ada", "alan"}; !equalStrings(got, want) {
		t.Errorf("Aliases = %v, want %v", got, want)
	}
	if resp.TotalCount != 2 {
		t.Errorf("TotalCount = %d, want 2", resp.TotalCount)
	}
	if resp.Truncated {
		t.Errorf("Truncated = true, want false")
	}
	// Header propagation: client.New with empty config still sets Accept; X-API-Key/Authorization
	// are empty when cfg carries no creds. Assertion documents the contract rather than requiring presence.
	_ = gotAPIKey
	_ = gotAuth
}

func TestFetchQualifiedContributors_EncodesProjectIDWithSpecialChars(t *testing.T) {
	var gotQuery string
	_, c := stubQualifiedServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"aliases": []string{}, "totalCount": 0, "truncated": false})
	})

	// "+" and space must be percent-encoded, not passed literally.
	_, err := fetchQualifiedContributors(context.Background(), c, "a+b c")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !strings.Contains(gotQuery, "project_id=a%2Bb+c") && !strings.Contains(gotQuery, "project_id=a%2Bb%20c") {
		t.Errorf("query = %q, want project_id encoded with + and space escaped", gotQuery)
	}
}

func TestFetchQualifiedContributors_EmptyResult(t *testing.T) {
	_, c := stubQualifiedServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"projectId":  "P",
			"aliases":    []string{},
			"totalCount": 0,
			"truncated":  false,
		})
	})

	resp, err := fetchQualifiedContributors(context.Background(), c, "P")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(resp.Aliases) != 0 {
		t.Errorf("Aliases = %v, want empty", resp.Aliases)
	}
}

func TestFetchQualifiedContributors_TruncatedFlagPassesThrough(t *testing.T) {
	_, c := stubQualifiedServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"projectId":  "P",
			"aliases":    []string{"a", "b", "c"},
			"totalCount": 3,
			"truncated":  true,
		})
	})

	resp, err := fetchQualifiedContributors(context.Background(), c, "P")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !resp.Truncated {
		t.Errorf("Truncated = false, want true")
	}
}

// Remap tests. These use remapQualifiedContributorsError directly rather than
// httptest so the mapping logic is covered without full network plumbing;
// the httptest path exercises client.Get → statusError → remap.

func TestRemapQualifiedContributorsError_Rewrites403ToManagerOf(t *testing.T) {
	in := &apierr.AuthError{HTTPStatus: 403, Message: "API error 403: forbidden"}
	out := remapQualifiedContributorsError(in, "P42")

	var authErr *apierr.AuthError
	if !errors.As(out, &authErr) {
		t.Fatalf("want *apierr.AuthError, got %T: %v", out, out)
	}
	if !strings.Contains(authErr.Message, "not a manager of project P42") {
		t.Errorf("rewritten message = %q, want substring %q", authErr.Message, "not a manager of project P42")
	}
	if authErr.HTTPStatus != 403 {
		t.Errorf("HTTPStatus = %d, want 403 preserved", authErr.HTTPStatus)
	}
}

func TestRemapQualifiedContributorsError_Passthrough401(t *testing.T) {
	in := &apierr.AuthError{HTTPStatus: 401, Message: "API error 401: not authenticated"}
	out := remapQualifiedContributorsError(in, "P42")

	if out != in {
		t.Errorf("401 should bubble unchanged, got %v (want same pointer as input)", out)
	}
	if strings.Contains(out.Error(), "not a manager of project") {
		t.Errorf("401 was rewritten with 403 hint: %q", out.Error())
	}
}

func TestRemapQualifiedContributorsError_Rewrites404(t *testing.T) {
	in := &apierr.NotFoundError{Message: "API error 404: project not found"}
	out := remapQualifiedContributorsError(in, "P42")

	var notFound *apierr.NotFoundError
	if !errors.As(out, &notFound) {
		t.Fatalf("want *apierr.NotFoundError, got %T: %v", out, out)
	}
	if !strings.Contains(notFound.Message, "project P42 not found") {
		t.Errorf("rewritten message = %q, want substring %q", notFound.Message, "project P42 not found")
	}
}

func TestRemapQualifiedContributorsError_Rewrites502(t *testing.T) {
	in := &apierr.ServerError{Status: 502, Message: "API error 502: bad gateway"}
	out := remapQualifiedContributorsError(in, "P42")

	var serverErr *apierr.ServerError
	if !errors.As(out, &serverErr) {
		t.Fatalf("want *apierr.ServerError, got %T: %v", out, out)
	}
	if serverErr.Status != 502 {
		t.Errorf("Status = %d, want 502", serverErr.Status)
	}
	if !strings.Contains(serverErr.Message, "scan temporarily unavailable") {
		t.Errorf("rewritten message = %q, want substring %q", serverErr.Message, "scan temporarily unavailable")
	}
}

func TestRemapQualifiedContributorsError_Passthrough500(t *testing.T) {
	in := &apierr.ServerError{Status: 500, Message: "API error 500: internal"}
	out := remapQualifiedContributorsError(in, "P42")

	if out != in {
		t.Errorf("500 should bubble unchanged, got %v (want same pointer as input)", out)
	}
	if strings.Contains(out.Error(), "scan temporarily unavailable") {
		t.Errorf("500 was rewritten with 502 hint: %q", out.Error())
	}
}

func TestRemapQualifiedContributorsError_PassthroughNonTyped(t *testing.T) {
	in := fmt.Errorf("network unreachable")
	out := remapQualifiedContributorsError(in, "P42")

	if out.Error() != in.Error() {
		t.Errorf("non-typed error should bubble unchanged, got %q", out.Error())
	}
}

// End-to-end error mapping over httptest — proves client.Get → statusError → remap pipeline.

func TestFetchQualifiedContributors_EndToEnd403Rewrite(t *testing.T) {
	_, c := stubQualifiedServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("forbidden"))
	})

	_, err := fetchQualifiedContributors(context.Background(), c, "P42")
	if err == nil {
		t.Fatal("expected error")
	}
	var authErr *apierr.AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("want *apierr.AuthError, got %T: %v", err, err)
	}
	if !strings.Contains(authErr.Message, "not a manager of project P42") {
		t.Errorf("message = %q, want substring %q", authErr.Message, "not a manager of project P42")
	}
}

func TestFetchQualifiedContributors_EndToEnd401Passthrough(t *testing.T) {
	_, c := stubQualifiedServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("unauthenticated"))
	})

	_, err := fetchQualifiedContributors(context.Background(), c, "P42")
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "not a manager of project") {
		t.Errorf("401 was rewritten: %q", err.Error())
	}
}

func TestFetchQualifiedContributors_EndToEnd404Rewrite(t *testing.T) {
	_, c := stubQualifiedServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("not found"))
	})

	_, err := fetchQualifiedContributors(context.Background(), c, "P42")
	if err == nil {
		t.Fatal("expected error")
	}
	var notFound *apierr.NotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("want *apierr.NotFoundError, got %T: %v", err, err)
	}
	if !strings.Contains(notFound.Message, "project P42 not found") {
		t.Errorf("message = %q, want substring %q", notFound.Message, "project P42 not found")
	}
}

func TestFetchQualifiedContributors_EndToEnd502Rewrite(t *testing.T) {
	_, c := stubQualifiedServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("bad gateway"))
	})

	_, err := fetchQualifiedContributors(context.Background(), c, "P42")
	if err == nil {
		t.Fatal("expected error")
	}
	var serverErr *apierr.ServerError
	if !errors.As(err, &serverErr) {
		t.Fatalf("want *apierr.ServerError, got %T: %v", err, err)
	}
	if serverErr.Status != 502 {
		t.Errorf("Status = %d, want 502", serverErr.Status)
	}
	if !strings.Contains(serverErr.Message, "scan temporarily unavailable") {
		t.Errorf("message = %q, want substring %q", serverErr.Message, "scan temporarily unavailable")
	}
}

// Rendering tests for the text-mode output. These cover the plan's
// handler-level test scenarios without needing to swap os.Stdout/os.Stderr.

func TestRenderQualifiedContributorsText_HappyPath(t *testing.T) {
	var stdout, stderr bytes.Buffer
	resp := qualifiedContributorsResponse{
		ProjectID: "P", Aliases: []string{"ada", "alan"}, TotalCount: 2, Truncated: false,
	}
	if err := renderQualifiedContributorsText(resp, &stdout, &stderr); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got, want := stdout.String(), "ada\nalan\n"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty", stderr.String())
	}
}

func TestRenderQualifiedContributorsText_EmptyResult(t *testing.T) {
	var stdout, stderr bytes.Buffer
	resp := qualifiedContributorsResponse{
		ProjectID: "P", Aliases: []string{}, TotalCount: 0, Truncated: false,
	}
	if err := renderQualifiedContributorsText(resp, &stdout, &stderr); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
	if got, want := stderr.String(), "No qualified contributors found.\n"; got != want {
		t.Errorf("stderr = %q, want %q", got, want)
	}
}

func TestRenderQualifiedContributorsText_NonEmptyTruncated(t *testing.T) {
	var stdout, stderr bytes.Buffer
	resp := qualifiedContributorsResponse{
		ProjectID: "P", Aliases: []string{"a", "b", "c"}, TotalCount: 3, Truncated: true,
	}
	if err := renderQualifiedContributorsText(resp, &stdout, &stderr); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got, want := stdout.String(), "a\nb\nc\n"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
	if !strings.Contains(stderr.String(), "warning: result truncated at 500 aliases") {
		t.Errorf("stderr = %q, want substring %q", stderr.String(), "warning: result truncated at 500 aliases")
	}
}

// TestRenderQualifiedContributorsText_EmptyAndTruncated covers the plan's
// "degenerate empty+truncated" scenario: both stderr lines fire, in order,
// with nothing on stdout. Plan reference: unit 2 test scenarios, edge case
// "Stub returns {aliases:[], totalCount:0, truncated:true}".
func TestRenderQualifiedContributorsText_EmptyAndTruncated(t *testing.T) {
	var stdout, stderr bytes.Buffer
	resp := qualifiedContributorsResponse{
		ProjectID: "P", Aliases: []string{}, TotalCount: 0, Truncated: true,
	}
	if err := renderQualifiedContributorsText(resp, &stdout, &stderr); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
	got := stderr.String()
	want := "No qualified contributors found.\nwarning: result truncated at 500 aliases\n"
	if got != want {
		t.Errorf("stderr = %q, want %q (both lines in that exact order)", got, want)
	}
}

func TestRenderQualifiedContributorsText_NilAliasesTreatedAsEmpty(t *testing.T) {
	// Guard against JSON `"aliases": null` decoding to nil slice. Range and len
	// both treat nil []string as empty, so the empty-result notice should fire.
	var stdout, stderr bytes.Buffer
	resp := qualifiedContributorsResponse{
		ProjectID: "P", Aliases: nil, TotalCount: 0, Truncated: false,
	}
	if err := renderQualifiedContributorsText(resp, &stdout, &stderr); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "No qualified contributors found.") {
		t.Errorf("stderr = %q, want substring %q", stderr.String(), "No qualified contributors found.")
	}
}

// =============================================================================
// project manager commitments — v2.3 wire-shape regression coverage
// =============================================================================

// fixtureV23ManagerCommitments points at the canonical v2.3 response body that
// also appears in v2-3-manager-commitments-list-response.md. The two are kept
// in sync by hand; if you edit one, edit both.
const fixtureV23ManagerCommitments = "../../internal/client/testdata/v2-3-manager-commitments-list-response.json"

// readFixture loads a testdata file relative to this test file.
func readFixture(t *testing.T, relPath string) []byte {
	t.Helper()
	abs, err := filepath.Abs(relPath)
	if err != nil {
		t.Fatalf("resolving fixture path: %v", err)
	}
	body, err := os.ReadFile(abs)
	if err != nil {
		t.Fatalf("reading fixture %s: %v", abs, err)
	}
	return body
}

// TestManagerCommitments_v23_ClientPostDecodesFixtureIntoMap pins the wire
// boundary the cobra handler relies on: a POST to the right path with the
// right body, and a successful decode of the v2.3 response into the same
// map[string]interface{} shape printListPost will hand to output.PrintJSON.
//
// What the test proves:
//   - The CLI POSTs to /api/v2/project/manager/commitments/list.
//   - The request body carries {"project_id": "<id>"} (v2.3 hard-required).
//   - client.Post decodes a v2.3 fixture without erroring or losing fields:
//     the decoded map deep-equals the same fixture decoded directly.
//
// What the test does NOT prove:
//   - That output.PrintJSON re-marshals the map without mutation. Both `got`
//     and `oracle` here decode the same bytes via encoding/json, so the
//     deep-equal is a stdlib-codec self-consistency check, not a stdout
//     observation. PrintJSON writes to os.Stdout via fmt.Println; capturing
//     that to assert byte-equality is heavier than the value it adds, given
//     map[string]interface{} → MarshalIndent is deterministic.
func TestManagerCommitments_v23_ClientPostDecodesFixtureIntoMap(t *testing.T) {
	body := readFixture(t, fixtureV23ManagerCommitments)

	var gotPath string
	var gotBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.Copy(w, bytes.NewReader(body))
	}))
	t.Cleanup(srv.Close)

	c := client.New(&config.Config{BaseURL: srv.URL})
	var got map[string]interface{}
	if err := c.Post(context.Background(), "/api/v2/project/manager/commitments/list",
		map[string]string{"project_id": "projA"}, &got); err != nil {
		t.Fatalf("client.Post: %v", err)
	}

	// Path + body propagation — v2.3 makes project_id required server-side; the
	// CLI forwards it in the body.
	if want := "/api/v2/project/manager/commitments/list"; gotPath != want {
		t.Errorf("path = %q, want %q", gotPath, want)
	}
	if got, want := gotBody, map[string]string{"project_id": "projA"}; !reflect.DeepEqual(got, want) {
		t.Errorf("request body = %v, want %v", got, want)
	}

	// Decode the fixture directly to confirm client.Post's decode path didn't
	// drop or reshape any fields relative to a vanilla json.Unmarshal — i.e.
	// the wire-to-map step the cobra handler depends on is faithful. This is
	// not a full stdout round trip; see the test docstring.
	var oracle map[string]interface{}
	if err := json.Unmarshal(body, &oracle); err != nil {
		t.Fatalf("decoding fixture oracle: %v", err)
	}
	if !reflect.DeepEqual(got, oracle) {
		t.Fatalf("response decoded shape diverged from fixture\n got:    %#v\n oracle: %#v", got, oracle)
	}
}

// TestManagerCommitments_v23_TextModeKeysPopulated guards the columns the cobra
// handler hands to output.PrintList. The current pairing is `submitted_by`
// (title) + `task_hash` (id). If the gateway ever drops or renames those
// top-level fields on either pending or assessed rows, text mode would print
// blank cells — the same pre-v2.3 silent failure that this PR fixes. Keep this
// test red-on-rename so the regression surfaces at CI rather than in the field.
func TestManagerCommitments_v23_TextModeKeysPopulated(t *testing.T) {
	body := readFixture(t, fixtureV23ManagerCommitments)
	var envelope map[string]interface{}
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decoding fixture: %v", err)
	}
	data, ok := envelope["data"].([]interface{})
	if !ok || len(data) < 2 {
		t.Fatalf("fixture must carry at least two rows; got %v", envelope["data"])
	}

	for i, raw := range data {
		row, ok := raw.(map[string]interface{})
		if !ok {
			t.Fatalf("row[%d] not an object: %T", i, raw)
		}
		// "submitted_by" and "task_hash" are the columns text mode reads.
		// Both are top-level required fields on ManagerCommitmentItem.
		if v, _ := row["submitted_by"].(string); v == "" {
			t.Errorf("row[%d] submitted_by empty; text mode would render blank title", i)
		}
		if v, _ := row["task_hash"].(string); v == "" {
			t.Errorf("row[%d] task_hash empty; text mode would render blank id", i)
		}
	}
}

// TestManagerCommitments_v23_FixtureMarkdownAndJSONInSync guards the dual
// fixture: the `.md` doc carries the response body inside an ```http code
// block and the `.json` sibling carries the same bytes for tests to read.
// Both are kept in sync by hand. This test extracts the JSON body from the
// `.md` and asserts structural equality against the `.json` so a future
// edit to one without the other fails CI rather than silently rotting the
// human-readable doc relative to the test source of truth.
func TestManagerCommitments_v23_FixtureMarkdownAndJSONInSync(t *testing.T) {
	mdPath := "../../internal/client/testdata/v2-3-manager-commitments-list-response.md"
	mdBytes, err := os.ReadFile(mdPath)
	if err != nil {
		t.Fatalf("reading markdown fixture: %v", err)
	}

	// Pull out the first ```http ... ``` block — that's the canonical response
	// envelope mirrored by the .json file.
	const fenceOpen = "```http"
	const fenceClose = "```"
	openIdx := strings.Index(string(mdBytes), fenceOpen)
	if openIdx == -1 {
		t.Fatalf("markdown fixture missing %q block", fenceOpen)
	}
	rest := string(mdBytes[openIdx+len(fenceOpen):])
	closeIdx := strings.Index(rest, fenceClose)
	if closeIdx == -1 {
		t.Fatalf("markdown fixture %q block is unclosed", fenceOpen)
	}
	httpBlock := rest[:closeIdx]

	// Inside the http block: HTTP headers, blank line, then the JSON body.
	// Split on the first blank line.
	parts := strings.SplitN(strings.TrimLeft(httpBlock, "\n"), "\n\n", 2)
	if len(parts) != 2 {
		t.Fatalf("markdown http block missing blank line between headers and body")
	}
	mdJSON := strings.TrimSpace(parts[1])

	jsonBytes := readFixture(t, fixtureV23ManagerCommitments)

	var fromMD, fromJSON map[string]interface{}
	if err := json.Unmarshal([]byte(mdJSON), &fromMD); err != nil {
		t.Fatalf("decoding markdown JSON body: %v\nbody was:\n%s", err, mdJSON)
	}
	if err := json.Unmarshal(jsonBytes, &fromJSON); err != nil {
		t.Fatalf("decoding json fixture: %v", err)
	}
	if !reflect.DeepEqual(fromMD, fromJSON) {
		t.Fatalf("markdown fixture body diverged from .json sibling — edit both when changing either\n .md:    %#v\n .json:  %#v", fromMD, fromJSON)
	}
}

// TestManagerCommitments_v23_AssessedRowCarriesEvidence tightens the JSON
// pass-through assertion to the specific shape the issue calls out: assessed
// rows must surface evidence + decision details. The fixture's first row is
// REWARDED with evidence, evidence_hash, assessed_by, and task_outcome —
// fail loudly if any of them go missing in the decoded envelope.
func TestManagerCommitments_v23_AssessedRowCarriesEvidence(t *testing.T) {
	body := readFixture(t, fixtureV23ManagerCommitments)
	var envelope map[string]interface{}
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decoding fixture: %v", err)
	}
	rows := envelope["data"].([]interface{})

	assessed, _ := rows[0].(map[string]interface{})
	content, _ := assessed["content"].(map[string]interface{})
	if content == nil {
		t.Fatal("assessed row missing content object")
	}
	if _, ok := content["evidence"].(map[string]interface{}); !ok {
		t.Errorf("assessed row content.evidence not a Tiptap document object: %T", content["evidence"])
	}
	for _, key := range []string{"task_evidence_hash", "commitment_status", "assessed_by", "task_outcome"} {
		v, _ := content[key].(string)
		if v == "" {
			t.Errorf("assessed row content.%s empty; v2.3 must surface this on assessed rows", key)
		}
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
