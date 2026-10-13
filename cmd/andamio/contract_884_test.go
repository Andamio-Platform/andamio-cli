package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Andamio-Platform/andamio-cli/internal/config"
	"github.com/Andamio-Platform/andamio-cli/internal/output"
	"github.com/spf13/cobra"
)

// =============================================================================
// andamio-api#884 — the CLI on both sides of the course and token route changes
// =============================================================================
//
// andamio-api#884 removes POST /v2/course/owner/course/create, drops tx_hash
// from the course register request, and moves the token registry read to
// /v2/project/user/token-registry/list. The CLI keeps every command working
// against the API before and after that change ships.

func contract884Env(t *testing.T, mux *http.ServeMux) {
	t.Helper()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	t.Setenv("HOME", t.TempDir())
	cfg := &config.Config{BaseURL: srv.URL, APIKey: "k", UserJWT: "u.jwt.value"}
	if err := config.Save(cfg); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	_ = output.SetFormat("json")
	t.Cleanup(func() { _ = output.SetFormat("text") })
}

func TestTokenList_UsesRegistryPath(t *testing.T) {
	var legacyHit bool
	mux := http.NewServeMux()
	mux.HandleFunc(tokenRegistryListPath, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"policy_id":"p","asset_name":"a"}]}`))
	})
	mux.HandleFunc(legacyTokenListPath, func(w http.ResponseWriter, r *http.Request) { legacyHit = true })
	contract884Env(t, mux)

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	if err := runTokenList(cmd, nil); err != nil {
		t.Fatalf("runTokenList: %v", err)
	}
	if legacyHit {
		t.Error("called the legacy path although the registry path answered")
	}
}

func TestTokenList_FallsBackToLegacyPathOn404(t *testing.T) {
	var legacyHit bool
	mux := http.NewServeMux()
	mux.HandleFunc(tokenRegistryListPath, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
	})
	mux.HandleFunc(legacyTokenListPath, func(w http.ResponseWriter, r *http.Request) {
		legacyHit = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"policy_id":"p","asset_name":"a"}]`))
	})
	contract884Env(t, mux)

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	if err := runTokenList(cmd, nil); err != nil {
		t.Fatalf("runTokenList: %v", err)
	}
	if !legacyHit {
		t.Error("did not fall back to the legacy path when the registry path was not served")
	}
}

// registerTestCmd carries the flags registerCourse reads, without touching the
// package-level commands' flag state.
func registerTestCmd(t *testing.T, args map[string]string) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{}
	for _, name := range []string{"course-id", "title", "description", "image-url", "video-url", "category", "tx-hash", "pending-tx-hash"} {
		cmd.Flags().String(name, "", "")
	}
	cmd.Flags().Bool("public", false, "")
	for k, v := range args {
		if err := cmd.Flags().Set(k, v); err != nil {
			t.Fatalf("set --%s: %v", k, err)
		}
	}
	cmd.SetContext(context.Background())
	return cmd
}

func captureRegister(t *testing.T) (*http.ServeMux, *map[string]interface{}, *bool) {
	t.Helper()
	body := map[string]interface{}{}
	var createHit bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/course/owner/course/register", func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"course_id":"c1"}`))
	})
	mux.HandleFunc("/api/v2/course/owner/course/create", func(w http.ResponseWriter, r *http.Request) { createHit = true })
	return mux, &body, &createHit
}

func TestCourseOwnerCreate_RegistersWithoutPendingTxHash(t *testing.T) {
	mux, body, createHit := captureRegister(t)
	contract884Env(t, mux)

	cmd := registerTestCmd(t, map[string]string{"course-id": "c1", "pending-tx-hash": "tx1", "title": "Intro"})
	if err := runCourseOwnerCreate(cmd, nil); err != nil {
		t.Fatalf("runCourseOwnerCreate: %v", err)
	}
	if *createHit {
		t.Error("called the removed create route")
	}
	if _, ok := (*body)["pending_tx_hash"]; ok {
		t.Error("sent pending_tx_hash, which the API no longer takes")
	}
	if (*body)["title"] != "Intro" || (*body)["course_id"] != "c1" {
		t.Errorf("register body = %v, want course_id c1 and title Intro", *body)
	}
}

func TestCourseOwnerCreate_TitleStaysOptional(t *testing.T) {
	mux, body, _ := captureRegister(t)
	contract884Env(t, mux)

	cmd := registerTestCmd(t, map[string]string{"course-id": "c1"})
	if err := runCourseOwnerCreate(cmd, nil); err != nil {
		t.Fatalf("create without --title must still run, as it did before: %v", err)
	}
	if _, ok := (*body)["title"]; ok {
		t.Error("sent an empty title")
	}
}

func TestCourseOwnerRegister_StillSendsTxHashWhenGiven(t *testing.T) {
	// The API before andamio-api#884 stores it; after, it ignores it.
	mux, body, _ := captureRegister(t)
	contract884Env(t, mux)

	cmd := registerTestCmd(t, map[string]string{"course-id": "c1", "title": "Intro", "tx-hash": "tx1"})
	if err := runCourseOwnerRegister(cmd, nil); err != nil {
		t.Fatalf("runCourseOwnerRegister: %v", err)
	}
	if (*body)["tx_hash"] != "tx1" {
		t.Errorf("tx_hash = %v, want tx1 while the API still stores it", (*body)["tx_hash"])
	}
}

func TestCourseOwnerRegister_StillRequiresTitle(t *testing.T) {
	mux, _, _ := captureRegister(t)
	contract884Env(t, mux)

	cmd := registerTestCmd(t, map[string]string{"course-id": "c1"})
	if err := runCourseOwnerRegister(cmd, nil); err == nil {
		t.Error("register without a title must still be refused")
	}
}
