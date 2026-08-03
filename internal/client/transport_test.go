package client

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Andamio-Platform/andamio-cli/internal/apierr"
	"github.com/Andamio-Platform/andamio-cli/internal/config"
)

// unreachableURL returns a URL whose port has nothing listening on it: a
// listener is opened to reserve a real port, then closed. More reliable than
// guessing a port number.
func unreachableURL(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	if err := ln.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return "http://" + addr
}

// The distinction #126 exists to create: a request that never reached the
// service is not the same outcome as an unclassified failure.
func TestTransport_UnreachableHostIsNetworkError(t *testing.T) {
	c := New(&config.Config{BaseURL: unreachableURL(t)})

	var result map[string]interface{}
	err := c.Get(context.Background(), "/anything", &result)
	if err == nil {
		t.Fatal("expected an error against a closed port")
	}

	var netErr *apierr.NetworkError
	if !errors.As(err, &netErr) {
		t.Fatalf("error is %T (%v), want *apierr.NetworkError", err, err)
	}
	if apierr.Kind(err) != apierr.KindUnreachable {
		t.Errorf("Kind = %q, want %q", apierr.Kind(err), apierr.KindUnreachable)
	}
}

func TestTransport_AllVerbsClassifyUnreachable(t *testing.T) {
	base := unreachableURL(t)
	c := New(&config.Config{BaseURL: base})
	ctx := context.Background()

	verbs := map[string]func() error{
		"GET":    func() error { var r map[string]interface{}; return c.Get(ctx, "/x", &r) },
		"POST":   func() error { var r map[string]interface{}; return c.Post(ctx, "/x", map[string]string{"a": "b"}, &r) },
		"PUT":    func() error { var r map[string]interface{}; return c.Put(ctx, "/x", map[string]string{"a": "b"}, &r) },
		"DELETE": func() error { return c.Delete(ctx, "/x", nil) },
	}

	for name, call := range verbs {
		t.Run(name, func(t *testing.T) {
			err := call()
			var netErr *apierr.NetworkError
			if !errors.As(err, &netErr) {
				t.Errorf("%s: error is %T (%v), want *apierr.NetworkError", name, err, err)
			}
		})
	}
}

// Cancellation is a distinct outcome from unreachability. Reporting the
// operator's own Ctrl-C as "could not reach the service" would send a caller
// chasing an outage that never happened.
func TestTransport_CancelledContextIsNotNetworkError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
	}))
	defer srv.Close()

	c := New(&config.Config{BaseURL: srv.URL})

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	var result map[string]interface{}
	err := c.Get(ctx, "/slow", &result)
	if err == nil {
		t.Fatal("expected an error from the cancelled request")
	}

	var netErr *apierr.NetworkError
	if errors.As(err, &netErr) {
		t.Errorf("cancellation was classified as a network error: %v", err)
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("error = %v, want context.Canceled", err)
	}
	if got := apierr.Kind(err); got != apierr.KindCanceled {
		t.Errorf("Kind = %q, want %q", got, apierr.KindCanceled)
	}
}

func TestTransport_DeadlineExceededIsNotNetworkError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
	}))
	defer srv.Close()

	c := New(&config.Config{BaseURL: srv.URL})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	var result map[string]interface{}
	err := c.Get(ctx, "/slow", &result)
	if err == nil {
		t.Fatal("expected an error from the expired deadline")
	}

	var netErr *apierr.NetworkError
	if errors.As(err, &netErr) {
		t.Errorf("deadline expiry was classified as a network error: %v", err)
	}
	if got := apierr.Kind(err); got != apierr.KindCanceled {
		t.Errorf("Kind = %q, want %q", got, apierr.KindCanceled)
	}
}

// A response that arrives carrying an error status is a status error, not a
// transport error. Transport wrapping must not swallow the taxonomy that
// already existed.
func TestTransport_StatusErrorsAreUnaffected(t *testing.T) {
	cases := []struct {
		status int
		want   string
	}{
		{http.StatusNotFound, apierr.KindNotFound},
		{http.StatusUnauthorized, apierr.KindAuth},
		{http.StatusForbidden, apierr.KindAuth},
		{http.StatusConflict, apierr.KindConflict},
		{http.StatusTooManyRequests, apierr.KindBackpressure},
		{http.StatusInternalServerError, apierr.KindServer},
	}

	for _, tc := range cases {
		t.Run(http.StatusText(tc.status), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(`{"message":"nope"}`))
			}))
			defer srv.Close()

			c := New(&config.Config{BaseURL: srv.URL})
			var result map[string]interface{}
			err := c.Get(context.Background(), "/x", &result)

			var netErr *apierr.NetworkError
			if errors.As(err, &netErr) {
				t.Errorf("status %d was classified as a network error", tc.status)
			}
			if got := apierr.Kind(err); got != tc.want {
				t.Errorf("status %d: Kind = %q, want %q", tc.status, got, tc.want)
			}
		})
	}
}

// The retry classifier reaches the underlying net error through NetworkError's
// Unwrap. Without that, wrapping would silently disable retries on exactly the
// failures most worth retrying.
func TestTransport_NetworkErrorRemainsRetryable(t *testing.T) {
	c := New(&config.Config{BaseURL: unreachableURL(t)})

	var result map[string]interface{}
	err := c.Get(context.Background(), "/x", &result)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !isRetryable(err) {
		t.Error("wrapped transport error is no longer retryable; NetworkError.Unwrap must expose the underlying net error")
	}
}
