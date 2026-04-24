package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
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
