package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Andamio-Platform/andamio-cli/internal/apierr"
	"github.com/Andamio-Platform/andamio-cli/internal/config"
)

// TestClient_StatusCodeToTypedError locks the contract between HTTP status codes
// and the typed error values surfaced by Get/Post/Put. A future refactor that
// drops a case or reorders the switch block fails here.
func TestClient_StatusCodeToTypedError(t *testing.T) {
	type assertFn func(t *testing.T, err error)

	// Match order follows switch-block order: 401, 403, 404, 409, 500 (fall-through).
	cases := []struct {
		name   string
		status int
		body   string
		assert assertFn
	}{
		{
			name:   "401 → AuthError",
			status: http.StatusUnauthorized,
			body:   "unauthorized",
			assert: func(t *testing.T, err error) {
				var authErr *apierr.AuthError
				if !errors.As(err, &authErr) {
					t.Fatalf("want *apierr.AuthError, got %T: %v", err, err)
				}
				if !strings.Contains(authErr.Message, "401") {
					t.Errorf("AuthError.Message should contain status 401, got %q", authErr.Message)
				}
			},
		},
		{
			name:   "403 → AuthError",
			status: http.StatusForbidden,
			body:   "forbidden",
			assert: func(t *testing.T, err error) {
				var authErr *apierr.AuthError
				if !errors.As(err, &authErr) {
					t.Fatalf("want *apierr.AuthError, got %T: %v", err, err)
				}
			},
		},
		{
			name:   "404 → NotFoundError",
			status: http.StatusNotFound,
			body:   "not found",
			assert: func(t *testing.T, err error) {
				var notFound *apierr.NotFoundError
				if !errors.As(err, &notFound) {
					t.Fatalf("want *apierr.NotFoundError, got %T: %v", err, err)
				}
			},
		},
		{
			name:   "409 → ConflictError",
			status: http.StatusConflict,
			body:   "course_module_code already exists in this course",
			assert: func(t *testing.T, err error) {
				var conflict *apierr.ConflictError
				if !errors.As(err, &conflict) {
					t.Fatalf("want *apierr.ConflictError, got %T: %v", err, err)
				}
				if !strings.Contains(conflict.Message, "409") {
					t.Errorf("ConflictError.Message should contain status 409, got %q", conflict.Message)
				}
				if !strings.Contains(conflict.Message, "course_module_code") {
					t.Errorf("ConflictError.Message should preserve body, got %q", conflict.Message)
				}
			},
		},
		{
			name:   "500 → plain error (fall-through; no typed match)",
			status: http.StatusInternalServerError,
			body:   "internal server error",
			assert: func(t *testing.T, err error) {
				var authErr *apierr.AuthError
				var notFound *apierr.NotFoundError
				var conflict *apierr.ConflictError
				if errors.As(err, &authErr) || errors.As(err, &notFound) || errors.As(err, &conflict) {
					t.Fatalf("500 must not match any typed error, got %T: %v", err, err)
				}
				if !strings.Contains(err.Error(), "500") {
					t.Errorf("generic error should contain status 500, got %q", err.Error())
				}
			},
		},
	}

	methods := []struct {
		name string
		call func(c *Client, path string) error
	}{
		{"Get", func(c *Client, path string) error {
			var out map[string]interface{}
			return c.Get(path, &out)
		}},
		{"Post", func(c *Client, path string) error {
			var out map[string]interface{}
			return c.Post(path, map[string]string{"k": "v"}, &out)
		}},
		{"Put", func(c *Client, path string) error {
			var out map[string]interface{}
			return c.Put(path, map[string]string{"k": "v"}, &out)
		}},
	}

	for _, m := range methods {
		for _, tc := range cases {
			t.Run(m.name+"/"+tc.name, func(t *testing.T) {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					http.Error(w, tc.body, tc.status)
				}))
				defer srv.Close()

				c := New(&config.Config{BaseURL: srv.URL})
				err := m.call(c, "/any/path")
				if err == nil {
					t.Fatalf("expected error for status %d, got nil", tc.status)
				}
				tc.assert(t, err)
			})
		}
	}
}

// TestClient_TypedErrorsSurviveWrapChain confirms that callers wrapping typed
// errors via fmt.Errorf(%w) can still extract them with errors.As. This is
// the property register-module relies on for mismatchError wrapping.
func TestClient_TypedErrorsSurviveWrapChain(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "course_module_code already exists", http.StatusConflict)
	}))
	defer srv.Close()

	c := New(&config.Config{BaseURL: srv.URL})
	var out map[string]interface{}
	rawErr := c.Post("/x", map[string]string{}, &out)
	if rawErr == nil {
		t.Fatal("expected error")
	}

	wrapped := fmt.Errorf("register failed: %w", rawErr)
	var conflict *apierr.ConflictError
	if !errors.As(wrapped, &conflict) {
		t.Fatalf("errors.As should unwrap *ConflictError through fmt.Errorf(%%w); got %T", wrapped)
	}
}

// TestClient_200OK_DecodesBody is the single happy-path assertion for each
// method. Not a behavioral focus of this PR but pins the non-error contract
// alongside the error-type contract above.
func TestClient_200OK_DecodesBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"ok": "yes"})
	}))
	defer srv.Close()

	c := New(&config.Config{BaseURL: srv.URL})

	var got map[string]string
	if err := c.Get("/x", &got); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got["ok"] != "yes" {
		t.Errorf("Get decoded %v, want ok=yes", got)
	}

	got = nil
	if err := c.Post("/x", map[string]string{"a": "b"}, &got); err != nil {
		t.Fatalf("Post: %v", err)
	}
	if got["ok"] != "yes" {
		t.Errorf("Post decoded %v, want ok=yes", got)
	}

	got = nil
	if err := c.Put("/x", map[string]string{"a": "b"}, &got); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if got["ok"] != "yes" {
		t.Errorf("Put decoded %v, want ok=yes", got)
	}
}
