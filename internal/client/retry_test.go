package client

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
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

// TestRetry_NonRetryable_401_403 covers the auth-error arm of the retry
// predicate. 401/403 are semantic failures ("re-auth"); they must never be
// retried regardless of how many times they recur.
func TestRetry_NonRetryable_401_403(t *testing.T) {
	for _, status := range []int{401, 403} {
		t.Run(fmt.Sprintf("status=%d", status), func(t *testing.T) {
			var hits int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				atomic.AddInt32(&hits, 1)
				http.Error(w, "not allowed", status)
			}))
			defer srv.Close()

			attempts, err := runRetry(t, srv, newTestRetryConfig())
			if err == nil {
				t.Fatal("expected error")
			}
			if attempts != 1 {
				t.Errorf("expected 1 attempt (no retry on %d), got %d", status, attempts)
			}
			var authErr *apierr.AuthError
			if !errors.As(err, &authErr) {
				t.Errorf("expected *apierr.AuthError, got %T", err)
			}
		})
	}
}

// TestRetry_NonRetryable_DeadlineExceeded covers the case where the
// caller-supplied context deadline fires during the first attempt.
// DeadlineExceeded must not be retried (caller asked us to stop).
func TestRetry_NonRetryable_DeadlineExceeded(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Stall the server forever so the client hits its deadline.
		select {
		case <-r.Context().Done():
		case <-time.After(10 * time.Second):
		}
	}))
	defer srv.Close()

	c := New(&config.Config{BaseURL: srv.URL})
	cfg := newTestRetryConfig()
	var attempts int32
	var out map[string]interface{}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := c.doWithRetry(ctx, cfg, func() error {
		atomic.AddInt32(&attempts, 1)
		return c.Post(ctx, "/x", map[string]string{}, &out)
	})
	if err == nil {
		t.Fatal("expected deadline error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("want errors.Is(err, context.DeadlineExceeded); got %v", err)
	}
	if attempts != 1 {
		t.Errorf("DeadlineExceeded must not retry; got %d attempts", attempts)
	}
}

// TestRetry_BackpressureError_RetryAfter_Parsed verifies that a 429 with a
// Retry-After hint in the response body is surfaced as
// *apierr.BackpressureError with RetryAfterSeconds populated, and that the
// retry classifier uses errors.As (not string matching) to detect it.
func TestRetry_BackpressureError_RetryAfter_Parsed(t *testing.T) {
	cases := []struct {
		name     string
		body     string
		wantSecs int
	}{
		{"integer seconds", "Retry-After: 2 slow down", 2},
		{"leading zero", "Retry-After: 02 slow down", 2},
		{"zero is allowed", "Retry-After: 0 slow down", 0},
		{"no hint", "too many requests", 0},
		{"http-date form (unsupported)", "Retry-After: Wed, 21 Oct 2015 07:28:00 GMT", 0},
		{"negative (rejected)", "Retry-After: -5 nope", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, tc.body, http.StatusTooManyRequests)
			}))
			defer srv.Close()

			c := New(&config.Config{BaseURL: srv.URL})
			var out map[string]interface{}
			err := c.Post(context.Background(), "/x", map[string]string{}, &out)
			if err == nil {
				t.Fatal("expected error")
			}
			var backpressure *apierr.BackpressureError
			if !errors.As(err, &backpressure) {
				t.Fatalf("expected *apierr.BackpressureError, got %T", err)
			}
			if backpressure.Status != 429 {
				t.Errorf("Status = %d, want 429", backpressure.Status)
			}
			if backpressure.RetryAfterSeconds != tc.wantSecs {
				t.Errorf("RetryAfterSeconds = %d, want %d", backpressure.RetryAfterSeconds, tc.wantSecs)
			}
			// Classifier must retry backpressure.
			if !isRetryable(err) {
				t.Errorf("BackpressureError must be retryable")
			}
		})
	}
}

// TestRetry_Backoff_HonorsRetryAfter verifies backoffDuration picks up the
// RetryAfterSeconds from BackpressureError.
func TestRetry_Backoff_HonorsRetryAfter(t *testing.T) {
	cfg := retryConfig{
		InitialBackoff: 1 * time.Millisecond,
		MaxBackoff:     500 * time.Millisecond,
		Jitter:         0,
	}
	// RetryAfterSeconds=0 → falls through to exponential (1ms).
	got := backoffDuration(cfg, 1, &apierr.BackpressureError{RetryAfterSeconds: 0})
	if got != 1*time.Millisecond {
		t.Errorf("with RetryAfter=0, expected exponential 1ms, got %v", got)
	}
	// Positive RetryAfterSeconds overrides exponential, still capped.
	got = backoffDuration(cfg, 1, &apierr.BackpressureError{RetryAfterSeconds: 10})
	if got != cfg.MaxBackoff {
		t.Errorf("RetryAfter: 10s should be capped at %v, got %v", cfg.MaxBackoff, got)
	}
}

// TestRetry_JitterScalesBackoff pins that non-zero jitter actually perturbs
// the backoff (neither zero nor deterministic). Production ships with
// Jitter=0.2; the retry-exhaustion tests use Jitter=0 for determinism, so
// this is the one place the jitter math is exercised.
func TestRetry_JitterScalesBackoff(t *testing.T) {
	cfg := retryConfig{
		InitialBackoff: 100 * time.Millisecond,
		MaxBackoff:     1 * time.Second,
		Jitter:         0.2,
	}
	seen := make(map[time.Duration]struct{})
	for i := 0; i < 50; i++ {
		d := backoffDuration(cfg, 1, nil)
		// ±20% of 100ms = [80ms, 120ms]. Allow a small tolerance for
		// float rounding.
		if d < 80*time.Millisecond || d > 120*time.Millisecond {
			t.Errorf("jitter produced duration %v, expected ~[80ms, 120ms]", d)
		}
		seen[d] = struct{}{}
	}
	if len(seen) < 5 {
		t.Errorf("expected jitter to produce varied durations, got %d distinct values", len(seen))
	}
}

// TestRetry_NetworkError_UnexpectedEOF verifies the io.ErrUnexpectedEOF
// branch of isNetworkLayerError. Simulates a server that starts responding
// then aborts mid-stream — Go wraps the EOF inside a *url.Error.
func TestRetry_NetworkError_UnexpectedEOF(t *testing.T) {
	// Server hijacks the connection and closes it immediately after
	// receiving the request. Client sees an unexpected EOF.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Fatal("server does not support hijack")
		}
		conn, _, err := hj.Hijack()
		if err != nil {
			t.Fatalf("hijack: %v", err)
		}
		_ = conn.Close()
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
		t.Fatal("expected network error")
	}
	if attempts != int32(cfg.MaxAttempts) {
		t.Errorf("EOF should be classified retryable and retried to exhaustion (%d); got %d attempts", cfg.MaxAttempts, attempts)
	}
}
