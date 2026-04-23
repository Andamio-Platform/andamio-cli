package client

import (
	"context"
	"errors"
	"io"
	"math/rand/v2"
	"net"
	"net/url"
	"syscall"
	"time"

	"github.com/Andamio-Platform/andamio-cli/internal/apierr"
)

// retryConfig tunes the retry wrapper. Not exported: callers use
// PostWithRetry (production defaults) or, in tests, newTestRetryConfig.
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
	// finishes and before the next attempt begins.
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
// asserts this per endpoint.
//
// Retries on network errors, *apierr.ServerError (5xx), and
// *apierr.BackpressureError (408/425/429, honoring Retry-After when set).
// Never retries *apierr.AuthError (401/403), *apierr.NotFoundError (404),
// *apierr.ConflictError (409), context.Canceled, or context.DeadlineExceeded.
//
// On exhaustion returns the last error unwrapped — typed errors survive
// errors.As through any outer fmt.Errorf(%w) wrap chain.
func (c *Client) PostWithRetry(ctx context.Context, path string, body interface{}, result interface{}) error {
	cfg := defaultRetryConfig()
	cfg.OnRetry = c.onRetry
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
// signals (408/425/429 via *apierr.BackpressureError). Everything else —
// including unknown error types — is not retried.
//
// Classification uses typed errors (errors.As) rather than string parsing
// so the retry predicate is not coupled to the statusError message format.
func isRetryable(err error) bool {
	if err == nil {
		return false
	}
	// Never retry when the caller has already cancelled.
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	// Semantic 4xx — never retry. Check these first so a future-added
	// field on these types cannot accidentally pass the retry predicate.
	var authErr *apierr.AuthError
	var notFound *apierr.NotFoundError
	var conflict *apierr.ConflictError
	if errors.As(err, &authErr) || errors.As(err, &notFound) || errors.As(err, &conflict) {
		return false
	}
	// 5xx — retry.
	var serverErr *apierr.ServerError
	if errors.As(err, &serverErr) {
		return true
	}
	// 408/425/429 — transient backpressure, retry.
	var backpressure *apierr.BackpressureError
	if errors.As(err, &backpressure) {
		return true
	}
	// Network-layer errors from http.Client.Do — retry.
	return isNetworkLayerError(err)
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
// error carries a Retry-After hint via *apierr.BackpressureError, that
// value overrides the exponential value (still capped and jittered).
func backoffDuration(cfg retryConfig, attempt int, err error) time.Duration {
	base := cfg.InitialBackoff << (attempt - 1)
	var backpressure *apierr.BackpressureError
	if errors.As(err, &backpressure) && backpressure.RetryAfterSeconds > 0 {
		base = time.Duration(backpressure.RetryAfterSeconds) * time.Second
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
