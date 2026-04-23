package client

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Andamio-Platform/andamio-cli/internal/apierr"
	"github.com/Andamio-Platform/andamio-cli/internal/config"
)

// newTestRetryConfig builds a config fast enough for unit tests. Exhaustion
// tests complete in well under 100ms with these values.
func newTestRetryConfig() retryConfig {
	return retryConfig{
		MaxAttempts:    3,
		InitialBackoff: 1 * time.Millisecond,
		MaxBackoff:     10 * time.Millisecond,
		Jitter:         0, // deterministic
	}
}

func runRetry(t *testing.T, srv *httptest.Server, cfg retryConfig) (int32, error) {
	t.Helper()
	c := New(&config.Config{BaseURL: srv.URL})
	var out map[string]interface{}
	var attempts int32
	err := c.doWithRetry(context.Background(), cfg, func() error {
		atomic.AddInt32(&attempts, 1)
		return c.Post(context.Background(), "/x", map[string]string{}, &out)
	})
	return atomic.LoadInt32(&attempts), err
}

func TestRetry_HappyPath_NoSleep(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":"yes"}`))
	}))
	defer srv.Close()

	attempts, err := runRetry(t, srv, newTestRetryConfig())
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if attempts != 1 {
		t.Errorf("expected 1 attempt, got %d", attempts)
	}
}

func TestRetry_500TwiceThen200(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&hits, 1)
		if n < 3 {
			http.Error(w, "oops", http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`{"ok":"yes"}`))
	}))
	defer srv.Close()

	attempts, err := runRetry(t, srv, newTestRetryConfig())
	if err != nil {
		t.Fatalf("expected success after retry, got %v", err)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestRetry_429WithRetryAfter(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&hits, 1)
		if n == 1 {
			http.Error(w, "Retry-After: 0 slow down", http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(`{"ok":"yes"}`))
	}))
	defer srv.Close()

	cfg := newTestRetryConfig()
	attempts, err := runRetry(t, srv, cfg)
	if err != nil {
		t.Fatalf("expected success after 429 retry, got %v", err)
	}
	if attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts)
	}
}

func TestRetry_408_425_429_AllRetry(t *testing.T) {
	for _, status := range []int{408, 425, 429} {
		t.Run(fmt.Sprintf("status=%d", status), func(t *testing.T) {
			var hits int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				n := atomic.AddInt32(&hits, 1)
				if n == 1 {
					http.Error(w, "backpressure", status)
					return
				}
				_, _ = w.Write([]byte(`{"ok":"yes"}`))
			}))
			defer srv.Close()

			attempts, err := runRetry(t, srv, newTestRetryConfig())
			if err != nil {
				t.Fatalf("expected success, got %v", err)
			}
			if attempts != 2 {
				t.Errorf("expected 2 attempts, got %d", attempts)
			}
		})
	}
}

func TestRetry_Exhaustion_503(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	cfg := newTestRetryConfig()
	attempts, err := runRetry(t, srv, cfg)
	if err == nil {
		t.Fatal("expected error after exhaustion")
	}
	if attempts != int32(cfg.MaxAttempts) {
		t.Errorf("expected %d attempts, got %d", cfg.MaxAttempts, attempts)
	}
	var serverErr *apierr.ServerError
	if !errors.As(err, &serverErr) {
		t.Errorf("expected *apierr.ServerError, got %T: %v", err, err)
	}
}

func TestRetry_NonRetryable_404(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	attempts, err := runRetry(t, srv, newTestRetryConfig())
	if err == nil {
		t.Fatal("expected error")
	}
	if attempts != 1 {
		t.Errorf("expected 1 attempt (no retry on 404), got %d", attempts)
	}
	var notFound *apierr.NotFoundError
	if !errors.As(err, &notFound) {
		t.Errorf("expected *apierr.NotFoundError, got %T", err)
	}
}

func TestRetry_NonRetryable_409(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		http.Error(w, "course_module_code already exists", http.StatusConflict)
	}))
	defer srv.Close()

	attempts, err := runRetry(t, srv, newTestRetryConfig())
	if err == nil {
		t.Fatal("expected error")
	}
	if attempts != 1 {
		t.Errorf("expected 1 attempt (no retry on 409), got %d; register-module recovery depends on this", attempts)
	}
	var conflict *apierr.ConflictError
	if !errors.As(err, &conflict) {
		t.Errorf("expected *apierr.ConflictError, got %T", err)
	}
}

func TestRetry_ContextCancelledMidBackoff(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	c := New(&config.Config{BaseURL: srv.URL})
	cfg := retryConfig{
		MaxAttempts:    5,
		InitialBackoff: 200 * time.Millisecond,
		MaxBackoff:     1 * time.Second,
		Jitter:         0,
	}

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel shortly after the first attempt — during the first backoff.
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	err := c.doWithRetry(ctx, cfg, func() error {
		var out map[string]interface{}
		return c.Post(context.Background(), "/x", map[string]string{}, &out)
	})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error after context cancel")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected errors.Is(err, context.Canceled), got %v", err)
	}
	if elapsed > 500*time.Millisecond {
		t.Errorf("cancel mid-backoff should return promptly; took %v", elapsed)
	}
}

func TestRetry_NetworkError_ConnectionRefused(t *testing.T) {
	// Start and immediately close a server so its address refuses connections.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close()

	c := New(&config.Config{BaseURL: url})
	cfg := newTestRetryConfig()
	var attempts int32
	var out map[string]interface{}
	err := c.doWithRetry(context.Background(), cfg, func() error {
		atomic.AddInt32(&attempts, 1)
		return c.Post(context.Background(), "/x", map[string]string{}, &out)
	})
	if err == nil {
		t.Fatal("expected network error")
	}
	if attempts != int32(cfg.MaxAttempts) {
		t.Errorf("expected network error to retry to exhaustion (%d), got %d", cfg.MaxAttempts, attempts)
	}
}

func TestRetry_DecodeFailureOn200_NoRetry(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		// 200 OK but malformed JSON.
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{not json`))
	}))
	defer srv.Close()

	c := New(&config.Config{BaseURL: srv.URL})
	cfg := newTestRetryConfig()
	var attempts int32
	var out map[string]interface{}
	err := c.doWithRetry(context.Background(), cfg, func() error {
		atomic.AddInt32(&attempts, 1)
		return c.Post(context.Background(), "/x", map[string]string{}, &out)
	})
	if err == nil {
		t.Fatal("expected decode error")
	}
	// The decode error comes back as an unknown error — classifier
	// defaults to non-retryable for unknown types, so attempts should be 1.
	// (This is the "default-safe" property.)
	if attempts != 1 {
		t.Errorf("post-200 decode failure must not retry; got %d attempts", attempts)
	}
}

func TestRetry_WrapChain_ErrorsAsStillWorks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	c := New(&config.Config{BaseURL: srv.URL})
	var out map[string]interface{}
	err := c.PostWithRetry(context.Background(), "/x", map[string]string{}, &out)
	if err == nil {
		t.Fatal("expected error")
	}

	// Double-wrap (mirrors registerOrRecoverModule → lookupTeacherModule).
	once := fmt.Errorf("failed to list modules: %w", err)
	twice := fmt.Errorf("could not locate it for recovery: %w", once)

	var serverErr *apierr.ServerError
	if !errors.As(twice, &serverErr) {
		t.Fatalf("errors.As should unwrap *ServerError through double fmt.Errorf(%%w); got %T", twice)
	}
	if serverErr.Status != 503 {
		t.Errorf("ServerError.Status = %d, want 503", serverErr.Status)
	}
}

func TestRetry_OnRetryCallbackFires(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&hits, 1)
		if n < 2 {
			http.Error(w, "oops", http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`{"ok":"yes"}`))
	}))
	defer srv.Close()

	c := New(&config.Config{BaseURL: srv.URL})
	var callbacks int32
	cfg := newTestRetryConfig()
	cfg.OnRetry = func(attempt int, wait time.Duration, err error) {
		atomic.AddInt32(&callbacks, 1)
		if err == nil {
			t.Errorf("OnRetry received nil err")
		}
	}
	var out map[string]interface{}
	err := c.doWithRetry(context.Background(), cfg, func() error {
		return c.Post(context.Background(), "/x", map[string]string{}, &out)
	})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if callbacks != 1 {
		t.Errorf("expected 1 retry callback, got %d", callbacks)
	}
}

// TestRetry_RetryAfterParse covers the tolerant parser — when the
// gateway body includes "Retry-After: N" the next backoff should be at
// least N seconds (capped at MaxBackoff).
func TestRetry_RetryAfterParseTolerant(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantN   int
		wantHit bool
	}{
		{"valid integer seconds", "Retry-After: 3 please try later", 3, true},
		{"leading zero", "Retry-After: 02 please", 2, true},
		{"zero is allowed", "Retry-After: 0 now", 0, true},
		{"missing colon form", "Retry-After 5 seconds", 0, false},
		{"unrelated body", "slow down", 0, false},
		{"http-date form (unsupported)", "Retry-After: Wed, 21 Oct 2015 07:28:00 GMT", 0, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := errors.New("API error 429: " + tc.body)
			got, ok := retryAfterFromError(err)
			if ok != tc.wantHit {
				t.Errorf("hit = %v, want %v (got=%v)", ok, tc.wantHit, got)
			}
			if ok && tc.name == "valid integer seconds" {
				if got != time.Duration(tc.wantN)*time.Second {
					t.Errorf("parsed %v, want %d s", got, tc.wantN)
				}
			}
			if !strings.Contains(err.Error(), tc.body) {
				t.Fatalf("test setup: body not in error message")
			}
		})
	}
}
