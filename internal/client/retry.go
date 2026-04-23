package client

import (
	"context"
	"errors"
	"io"
	"math/rand/v2"
	"net"
	"net/url"
	"strconv"
	"syscall"
	"time"

	"github.com/Andamio-Platform/andamio-cli/internal/apierr"
)

// retryConfig tunes the retry wrapper. Not exported: callers use
// PostWithRetry (production defaults) or, in tests, the test-only
// newTestRetryConfig helper that builds a faster config.
type retryConfig struct {
	// MaxAttempts is the total number of attempts (initial + retries).
	// A value of 3 means: initial, retry 1, retry 2.
	MaxAttempts int
	// InitialBackoff is the sleep before the first retry.
	InitialBackoff time.Duration
	// MaxBackoff caps individual backoff durations (applied after exponential
	// growth and after any server-supplied Retry-After).
	MaxBackoff time.Duration
	// Jitter is the symmetric fraction (0.0 - 1.0) of randomness applied to
	// each backoff. 0.2 means the sleep is scaled by a random value in
	// [0.8, 1.2].
	Jitter float64
	// OnRetry is optional. When set, called once per retry after the sleep
	// finishes and before the next attempt begins. Lets the cobra layer
	// log retries to stderr without coupling internal/client to
	// internal/output.
	OnRetry func(attempt int, wait time.Duration, err error)
}

// defaultRetryConfig is the production-tuned config used by
// PostWithRetry / future *WithRetry methods.
func defaultRetryConfig() retryConfig {
	return retryConfig{
		MaxAttempts:    3,
		InitialBackoff: 250 * time.Millisecond,
		MaxBackoff:     5 * time.Second,
		Jitter:         0.2,
	}
}

// PostWithRetry wraps Post with bounded retries on transient failures. Safe
// only for idempotent (side-effect-free) POST endpoints — the caller
// asserts this per endpoint. See retry.isRetryable for the retry predicate.
//
// On exhaustion returns the last error unwrapped (it preserves typed errors
// via errors.As through any outer fmt.Errorf(%w) wrap chain).
func (c *Client) PostWithRetry(ctx context.Context, path string, body interface{}, result interface{}) error {
	cfg := defaultRetryConfig()
	return c.doWithRetry(ctx, cfg, func() error {
		return c.Post(ctx, path, body, result)
	})
}

// doWithRetry is the retry core. Runs fn up to cfg.MaxAttempts times,
// classifying each error and sleeping between attempts.
func (c *Client) doWithRetry(ctx context.Context, cfg retryConfig, fn func() error) error {
	if cfg.MaxAttempts < 1 {
		cfg.MaxAttempts = 1
	}
	var lastErr error
	for attempt := 1; attempt <= cfg.MaxAttempts; attempt++ {
		err := fn()
		if err == nil {
			return nil
		}
		lastErr = err
		if !isRetryable(err) {
			return err
		}
		if attempt == cfg.MaxAttempts {
			return err
		}
		wait := backoffDuration(cfg, attempt, err)
		select {
		case <-ctx.Done():
			// Caller cancelled mid-backoff. Surface the context error,
			// but preserve the underlying err in case something wants it.
			return errors.Join(ctx.Err(), err)
		case <-time.After(wait):
		}
		if cfg.OnRetry != nil {
			cfg.OnRetry(attempt+1, wait, err)
		}
	}
	return lastErr
}

// isRetryable decides whether err is worth retrying. Conservative: only
// network-layer errors, 5xx responses, and specific 4xx backpressure
// signals (408/425/429). Everything else — including unknown error types —
// is not retried.
func isRetryable(err error) bool {
	if err == nil {
		return false
	}
	// Never retry when the caller has already cancelled.
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	// Typed errors from the client:
	//   - ServerError: 5xx, retry.
	//   - AuthError / NotFoundError / ConflictError: non-retryable
	//     (401/403/404/409 have semantic meaning).
	//   - StatusError (backpressure 408/425/429): retry.
	var serverErr *apierr.ServerError
	if errors.As(err, &serverErr) {
		return true
	}
	var authErr *apierr.AuthError
	var notFound *apierr.NotFoundError
	var conflict *apierr.ConflictError
	if errors.As(err, &authErr) || errors.As(err, &notFound) || errors.As(err, &conflict) {
		return false
	}
	if isBackpressureError(err) {
		return true
	}
	// Network-layer errors: connection refused, reset, unexpected EOF,
	// DNS, etc. Raw errors from http.Client.Do fall into these buckets.
	return isNetworkLayerError(err)
}

// isBackpressureError returns true for 408 / 425 / 429 responses. These
// arrive as plain `errors.New("API error %d: ...")` today (no typed
// variant) — parse the status out of the message prefix.
func isBackpressureError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	for _, prefix := range []string{"API error 408", "API error 425", "API error 429"} {
		if len(msg) >= len(prefix) && msg[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

// isNetworkLayerError returns true for errors that represent a transport
// failure (vs. an HTTP-status error or a marshal/decode error). Errors
// returned by http.Client.Do typically wrap *url.Error around the
// underlying net.Error / syscall.Errno.
func isNetworkLayerError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
		return true
	}
	if errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.ECONNREFUSED) {
		return true
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		// Context errors already filtered in isRetryable; anything else
		// wrapped in a *url.Error is transport-level.
		if errors.Is(urlErr.Err, context.Canceled) || errors.Is(urlErr.Err, context.DeadlineExceeded) {
			return false
		}
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	return false
}

// backoffDuration computes the wait before the next attempt. Applies
// exponential growth, capped at MaxBackoff, then symmetric jitter. If the
// error carries a Retry-After hint (429), that value overrides the
// exponential value (still capped and jittered).
func backoffDuration(cfg retryConfig, attempt int, err error) time.Duration {
	base := cfg.InitialBackoff << (attempt - 1)
	if retryAfter, ok := retryAfterFromError(err); ok {
		base = retryAfter
	}
	if base > cfg.MaxBackoff {
		base = cfg.MaxBackoff
	}
	if cfg.Jitter > 0 {
		// math/rand/v2 top-level functions are goroutine-safe.
		factor := 1 + (rand.Float64()*2-1)*cfg.Jitter
		base = time.Duration(float64(base) * factor)
	}
	if base < 0 {
		base = 0
	}
	return base
}

// retryAfterFromError parses a Retry-After hint embedded in a 429 error
// message. The gateway's 429 body may optionally include "Retry-After: N"
// (seconds); if absent, the classifier falls through to exponential
// backoff. Returns (duration, true) on a successful parse.
//
// This is a conservative parse: only integer seconds from a body that
// contains "Retry-After:" are accepted. HTTP Retry-After can also be a
// date — we do not support that form here; the fallback schedule
// handles it gracefully.
func retryAfterFromError(err error) (time.Duration, bool) {
	if err == nil {
		return 0, false
	}
	msg := err.Error()
	idx := -1
	for i := 0; i+len("Retry-After:") <= len(msg); i++ {
		if msg[i:i+len("Retry-After:")] == "Retry-After:" {
			idx = i + len("Retry-After:")
			break
		}
	}
	if idx < 0 {
		return 0, false
	}
	// Skip whitespace.
	for idx < len(msg) && (msg[idx] == ' ' || msg[idx] == '\t') {
		idx++
	}
	// Read digits.
	end := idx
	for end < len(msg) && msg[end] >= '0' && msg[end] <= '9' {
		end++
	}
	if end == idx {
		return 0, false
	}
	n, err := strconv.Atoi(msg[idx:end])
	if err != nil || n < 0 {
		return 0, false
	}
	return time.Duration(n) * time.Second, true
}
