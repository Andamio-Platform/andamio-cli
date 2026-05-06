package client

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
	"github.com/Andamio-Platform/andamio-cli/internal/config"
)

// TestClient_StatusCodeToTypedError locks the contract between HTTP status codes
// and the typed error values surfaced by Get/Post/Put. A future refactor that
// drops a case or reorders the switch block fails here.
func TestClient_StatusCodeToTypedError(t *testing.T) {
	type assertFn func(t *testing.T, err error)

	// Match order follows switch-block order: 401, 403, 404, 409, 5xx, generic.
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
				if authErr.HTTPStatus != 401 {
					t.Errorf("AuthError.HTTPStatus = %d, want 401", authErr.HTTPStatus)
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
				if !strings.Contains(authErr.Message, "403") {
					t.Errorf("AuthError.Message should contain status 403, got %q", authErr.Message)
				}
				if authErr.HTTPStatus != 403 {
					t.Errorf("AuthError.HTTPStatus = %d, want 403", authErr.HTTPStatus)
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
				if !strings.Contains(notFound.Message, "404") {
					t.Errorf("NotFoundError.Message should contain status 404, got %q", notFound.Message)
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
			name:   "500 → ServerError",
			status: http.StatusInternalServerError,
			body:   "internal server error",
			assert: func(t *testing.T, err error) {
				var serverErr *apierr.ServerError
				if !errors.As(err, &serverErr) {
					t.Fatalf("want *apierr.ServerError, got %T: %v", err, err)
				}
				if serverErr.Status != 500 {
					t.Errorf("ServerError.Status = %d, want 500", serverErr.Status)
				}
				if !strings.Contains(serverErr.Message, "500") {
					t.Errorf("ServerError.Message should contain status 500, got %q", serverErr.Message)
				}
			},
		},
		{
			name:   "502 → ServerError",
			status: http.StatusBadGateway,
			body:   "bad gateway",
			assert: func(t *testing.T, err error) {
				var serverErr *apierr.ServerError
				if !errors.As(err, &serverErr) {
					t.Fatalf("want *apierr.ServerError, got %T: %v", err, err)
				}
				if serverErr.Status != 502 {
					t.Errorf("ServerError.Status = %d, want 502", serverErr.Status)
				}
			},
		},
		{
			name:   "503 → ServerError",
			status: http.StatusServiceUnavailable,
			body:   "service unavailable",
			assert: func(t *testing.T, err error) {
				var serverErr *apierr.ServerError
				if !errors.As(err, &serverErr) {
					t.Fatalf("want *apierr.ServerError, got %T: %v", err, err)
				}
			},
		},
		{
			name:   "418 → plain error (no typed match)",
			status: http.StatusTeapot,
			body:   "short and stout",
			assert: func(t *testing.T, err error) {
				var authErr *apierr.AuthError
				var notFound *apierr.NotFoundError
				var conflict *apierr.ConflictError
				var serverErr *apierr.ServerError
				if errors.As(err, &authErr) || errors.As(err, &notFound) || errors.As(err, &conflict) || errors.As(err, &serverErr) {
					t.Fatalf("418 must not match any typed error, got %T: %v", err, err)
				}
				if !strings.Contains(err.Error(), "418") {
					t.Errorf("generic error should contain status 418, got %q", err.Error())
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
			return c.Get(context.Background(), path, &out)
		}},
		{"Post", func(c *Client, path string) error {
			var out map[string]interface{}
			return c.Post(context.Background(), path, map[string]string{"k": "v"}, &out)
		}},
		{"Put", func(c *Client, path string) error {
			var out map[string]interface{}
			return c.Put(context.Background(), path, map[string]string{"k": "v"}, &out)
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
// errors via fmt.Errorf(%w) can still extract them with errors.As. Register-module
// relies on this for ConflictError; the retry classifier relies on it for
// ServerError.
func TestClient_TypedErrorsSurviveWrapChain(t *testing.T) {
	t.Run("ConflictError", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "course_module_code already exists", http.StatusConflict)
		}))
		defer srv.Close()

		c := New(&config.Config{BaseURL: srv.URL})
		var out map[string]interface{}
		rawErr := c.Post(context.Background(), "/x", map[string]string{}, &out)
		if rawErr == nil {
			t.Fatal("expected error")
		}

		wrapped := fmt.Errorf("register failed: %w", rawErr)
		var conflict *apierr.ConflictError
		if !errors.As(wrapped, &conflict) {
			t.Fatalf("errors.As should unwrap *ConflictError through fmt.Errorf(%%w); got %T", wrapped)
		}
	})

	t.Run("ServerError double-wrap", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "upstream failed", http.StatusBadGateway)
		}))
		defer srv.Close()

		c := New(&config.Config{BaseURL: srv.URL})
		var out map[string]interface{}
		rawErr := c.Post(context.Background(), "/x", map[string]string{}, &out)
		if rawErr == nil {
			t.Fatal("expected error")
		}

		// Two-level wrap mirrors registerOrRecoverModule → lookupTeacherModule.
		once := fmt.Errorf("failed to list modules: %w", rawErr)
		twice := fmt.Errorf("could not locate it for recovery: %w", once)

		var serverErr *apierr.ServerError
		if !errors.As(twice, &serverErr) {
			t.Fatalf("errors.As should unwrap *ServerError through double fmt.Errorf(%%w); got %T", twice)
		}
		if serverErr.Status != 502 {
			t.Errorf("ServerError.Status = %d, want 502", serverErr.Status)
		}
	})
}

// TestClient_200OK_DecodesBody is the single happy-path assertion for each
// method.
func TestClient_200OK_DecodesBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"ok": "yes"})
	}))
	defer srv.Close()

	c := New(&config.Config{BaseURL: srv.URL})
	ctx := context.Background()

	var got map[string]string
	if err := c.Get(ctx, "/x", &got); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got["ok"] != "yes" {
		t.Errorf("Get decoded %v, want ok=yes", got)
	}

	got = nil
	if err := c.Post(ctx, "/x", map[string]string{"a": "b"}, &got); err != nil {
		t.Fatalf("Post: %v", err)
	}
	if got["ok"] != "yes" {
		t.Errorf("Post decoded %v, want ok=yes", got)
	}

	got = nil
	if err := c.Put(ctx, "/x", map[string]string{"a": "b"}, &got); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if got["ok"] != "yes" {
		t.Errorf("Put decoded %v, want ok=yes", got)
	}
}

// TestClient_ContextCancel_PreHeaders confirms that cancelling ctx before the
// server responds aborts the in-flight request promptly instead of waiting for
// the 30s wall-clock timeout.
func TestClient_ContextCancel_PreHeaders(t *testing.T) {
	serverReceivedCancel := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
			// Server observed the client cancelling — this is the assertion we want.
			serverReceivedCancel <- struct{}{}
			return
		case <-time.After(10 * time.Second):
			// Failure path: the server never got a cancel signal.
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	c := New(&config.Config{BaseURL: srv.URL})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	var out map[string]interface{}
	err := c.Get(ctx, "/slow", &out)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error from ctx timeout, got nil")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("want errors.Is(err, context.DeadlineExceeded); got %v", err)
	}
	if elapsed > 5*time.Second {
		t.Errorf("ctx cancel should abort promptly; took %v", elapsed)
	}

	select {
	case <-serverReceivedCancel:
		// Good: server saw the cancel.
	case <-time.After(500 * time.Millisecond):
		t.Error("server never observed request cancellation")
	}
}

// TestClient_ContextCancel_MidBody covers the mid-body-read cancellation path:
// server sends headers, then stalls before writing body. Client ctx timeout
// should still fire promptly.
func TestClient_ContextCancel_MidBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Flush headers, then stall before the body.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		select {
		case <-r.Context().Done():
			return
		case <-time.After(10 * time.Second):
			_, _ = w.Write([]byte(`{"ok":"too-late"}`))
		}
	}))
	defer srv.Close()

	c := New(&config.Config{BaseURL: srv.URL})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	var out map[string]interface{}
	err := c.Get(ctx, "/slow-body", &out)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error from ctx timeout mid-body, got nil")
	}
	if elapsed > 5*time.Second {
		t.Errorf("ctx cancel during body read should abort promptly; took %v", elapsed)
	}
}

// TestClient_Delete_204_NilResult pins the canonical DELETE shape: 204 No
// Content with the caller passing nil — the method should NOT attempt to
// decode (no body to decode anyway) and should return nil. This is the
// path every current caller takes (DELETE /v2/keys/{id}).
func TestClient_Delete_204_NilResult(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := New(&config.Config{BaseURL: srv.URL})
	if err := c.Delete(context.Background(), "/resource/abc", nil); err != nil {
		t.Errorf("Delete with 204+nil-result should succeed; got %v", err)
	}
}

// TestClient_Delete_200WithBody_DecodesIntoResult pins the second shape
// the round-2 signature change exists to support: a DELETE that returns
// 200 with a JSON body (e.g., a future revoke endpoint that emits an
// audit-log id). The decode branch (`if result != nil`) is otherwise
// dead from a test perspective since the only production caller passes
// nil.
func TestClient_Delete_200WithBody_DecodesIntoResult(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"audit_id":"audit-42","status":"revoked"}`))
	}))
	defer srv.Close()

	c := New(&config.Config{BaseURL: srv.URL})
	var got struct {
		AuditID string `json:"audit_id"`
		Status  string `json:"status"`
	}
	if err := c.Delete(context.Background(), "/resource/abc", &got); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if got.AuditID != "audit-42" || got.Status != "revoked" {
		t.Errorf("decode = %+v, want {AuditID:audit-42 Status:revoked}", got)
	}
}

// TestClient_Delete_404_TypedError verifies non-2xx status codes still
// produce the typed errors the rest of the client surfaces. 404 should
// be *apierr.NotFoundError so callers can `errors.As` against it for the
// "not found OR not owned" disambiguation that `dev keys delete` does.
func TestClient_Delete_404_TypedError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not found"}`))
	}))
	defer srv.Close()

	c := New(&config.Config{BaseURL: srv.URL})
	err := c.Delete(context.Background(), "/resource/missing", nil)
	if err == nil {
		t.Fatal("expected error on 404")
	}
	var nfErr *apierr.NotFoundError
	if !errors.As(err, &nfErr) {
		t.Errorf("expected *apierr.NotFoundError, got %T: %v", err, err)
	}
}
