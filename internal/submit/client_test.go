package submit

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A wrong submit path on Blockfrost answers 301 to its homepage, which
// answers 200. Following that redirect used to turn a failed submit into
// a success. The redirect must be reported as an error, and the redirect
// target must never be requested.
func TestSubmitTransactionRedirectIsAnError(t *testing.T) {
	homepageHit := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			homepageHit = true
			w.Write([]byte("<html>homepage</html>"))
			return
		}
		http.Redirect(w, r, "/", http.StatusMovedPermanently)
	}))
	defer srv.Close()

	body, err := SubmitTransaction(srv.URL+"/api/tx/submit", "84a0", nil)
	if err == nil {
		t.Fatalf("expected an error, got success with body %q", body)
	}
	if !strings.Contains(err.Error(), "redirected (301") {
		t.Errorf("error should name the redirect, got: %v", err)
	}
	if homepageHit {
		t.Error("redirect was followed: the redirect target was requested")
	}
}

func TestSubmitTransactionSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if got := r.Header.Get("Content-Type"); got != "application/cbor" {
			t.Errorf("Content-Type = %q, want application/cbor", got)
		}
		if got := r.Header.Get("project_id"); got != "preprodABC" {
			t.Errorf("project_id = %q, want preprodABC", got)
		}
		w.Write([]byte(`"abc123"`))
	}))
	defer srv.Close()

	body, err := SubmitTransaction(srv.URL+"/api/v0/tx/submit", "84a0", []string{"project_id: preprodABC"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(body) != `"abc123"` {
		t.Errorf("body = %q, want \"abc123\"", body)
	}
}
