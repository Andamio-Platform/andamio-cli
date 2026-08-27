package client

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/Andamio-Platform/andamio-cli/internal/apierr"
	"github.com/Andamio-Platform/andamio-cli/internal/config"
)

// Get must accept the whole 2xx range like Post/Put/Delete. The gateway's
// merged read endpoints answer 206 Partial Content with the normal data plus
// meta.warning when one backend is degraded; treating that as an error threw
// the data away (#157).
func TestClient_Get_206PartialContent_DecodesBody(t *testing.T) {
	body, err := os.ReadFile("testdata/v2-5-partial-content-response.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	c := New(&config.Config{BaseURL: srv.URL})
	var got map[string]interface{}
	if err := c.Get(context.Background(), "/api/v2/project/user/project/x", &got); err != nil {
		t.Fatalf("Get on 206 returned error: %v", err)
	}
	data, _ := got["data"].(map[string]interface{})
	if data["source"] != "chain_only" {
		t.Errorf("data not decoded from 206 body: %v", got)
	}
	meta, _ := got["meta"].(map[string]interface{})
	if meta["warning"] != "DB API unavailable, showing on-chain data only" {
		t.Errorf("meta.warning not preserved: %v", got)
	}
}

func TestClient_Get_204NoContent_NoDecodeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := New(&config.Config{BaseURL: srv.URL})
	var got map[string]interface{}
	if err := c.Get(context.Background(), "/x", &got); err != nil {
		t.Fatalf("Get on 204 returned error: %v", err)
	}
	if got != nil {
		t.Errorf("204 should leave result untouched, got %v", got)
	}
}

// Widening to 2xx must not loosen the error side: a 404 is still not_found.
func TestClient_Get_404StillNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"code":"NOT_FOUND","message":"nope"}}`, http.StatusNotFound)
	}))
	defer srv.Close()

	c := New(&config.Config{BaseURL: srv.URL})
	var got map[string]interface{}
	err := c.Get(context.Background(), "/x", &got)
	var nf *apierr.NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("want *apierr.NotFoundError, got %T: %v", err, err)
	}
	if apierr.Kind(err) != apierr.KindNotFound {
		t.Errorf("Kind = %q", apierr.Kind(err))
	}
}
