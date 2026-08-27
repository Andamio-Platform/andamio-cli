package client

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Andamio-Platform/andamio-cli/internal/apierr"
	"github.com/Andamio-Platform/andamio-cli/internal/config"
)

// The gateway's real envelope for a tier cap — andamio-api WriteErrorResponse
// pretty-prints it, which is why the decoder reads the raw body rather than
// the truncated message.
const tierLimitEnvelope = `{
    "error": {
        "code": "tier_limit_exceeded",
        "message": "maximum API key limit (1) reached for your tier; revoke an existing key or upgrade your subscription",
        "details": ""
    }
}`

func TestDecodeGatewayErrorCode(t *testing.T) {
	cases := []struct {
		name        string
		body        string
		wantOK      bool
		wantCode    string
		wantMessage string
		wantDetails string
	}{
		{"nested envelope", tierLimitEnvelope, true, "tier_limit_exceeded", "maximum API key limit (1) reached for your tier; revoke an existing key or upgrade your subscription", ""},
		{"nested with details", `{"error":{"code":"x","message":"m","details":"d"}}`, true, "x", "m", "d"},
		{"flat string error", `{"error":"tier_limit_exceeded"}`, true, "tier_limit_exceeded", "", ""},
		{"flat legacy with message sibling", `{"error":"tier_limit_exceeded","message":"tier cap reached"}`, true, "tier_limit_exceeded", "", ""},
		{"whitespace trimmed", `{"error":{"code":"  x  ","message":" m "}}`, true, "x", "m", ""},
		{"empty body", ``, false, "", "", ""},
		{"whitespace body", "  \n ", false, "", "", ""},
		{"html body", `<html><body>502 Bad Gateway</body></html>`, false, "", "", ""},
		{"plain text body", "backpressure\n", false, "", "", ""},
		{"no error field", `{"message":"stub"}`, false, "", "", ""},
		{"null error", `{"error":null}`, false, "", "", ""},
		{"blank code", `{"error":{"code":"   ","message":"m"}}`, false, "", "", ""},
		{"missing code", `{"error":{"message":"m"}}`, false, "", "", ""},
		{"array error", `{"error":["x"]}`, false, "", "", ""},
		{"numeric error", `{"error":42}`, false, "", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, message, details, ok := decodeGatewayErrorCode([]byte(tc.body))
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v (code=%q)", ok, tc.wantOK, code)
			}
			if code != tc.wantCode || message != tc.wantMessage || details != tc.wantDetails {
				t.Errorf("got (%q, %q, %q), want (%q, %q, %q)", code, message, details, tc.wantCode, tc.wantMessage, tc.wantDetails)
			}
		})
	}
}

// Classification keys on the body code, on any 4xx, before the status switch.
// Every other body on every other status must land exactly where it did
// before — the table pairs each tier_limit case with the untouched sibling.
func TestStatusError_TierLimitByBodyCode(t *testing.T) {
	longDetails := strings.Repeat("d", 700)
	cases := []struct {
		name   string
		status int
		body   string
		assert func(t *testing.T, err error)
	}{
		{
			"429 + coded envelope → TierLimitError", http.StatusTooManyRequests, tierLimitEnvelope,
			func(t *testing.T, err error) {
				var tl *apierr.TierLimitError
				if !errors.As(err, &tl) {
					t.Fatalf("want *apierr.TierLimitError, got %T: %v", err, err)
				}
				if tl.Status != 429 || tl.Code != "tier_limit_exceeded" {
					t.Errorf("Status/Code = %d/%q", tl.Status, tl.Code)
				}
				if !strings.Contains(tl.Message, "revoke an existing key or upgrade") {
					t.Errorf("Message should carry the gateway sentence verbatim, got %q", tl.Message)
				}
				if !strings.Contains(err.Error(), "tier_limit_exceeded") {
					t.Errorf("Error() must keep the stable code string, got %q", err.Error())
				}
			},
		},
		{
			"403 + coded envelope → TierLimitError (product-circle#304 forward-compat)", http.StatusForbidden, tierLimitEnvelope,
			func(t *testing.T, err error) {
				var tl *apierr.TierLimitError
				if !errors.As(err, &tl) {
					t.Fatalf("want *apierr.TierLimitError on 403, got %T: %v", err, err)
				}
				var auth *apierr.AuthError
				if errors.As(err, &auth) {
					t.Error("a coded 403 must not also be an AuthError")
				}
			},
		},
		{
			"429 + flat coded body → TierLimitError", http.StatusTooManyRequests, `{"error":"tier_limit_exceeded"}`,
			func(t *testing.T, err error) {
				var tl *apierr.TierLimitError
				if !errors.As(err, &tl) {
					t.Fatalf("want *apierr.TierLimitError, got %T: %v", err, err)
				}
				if tl.Message == "" {
					t.Error("Message should fall back to the body when the flat shape carries none")
				}
			},
		},
		{
			"429 + {message:stub} → BackpressureError (unchanged)", http.StatusTooManyRequests, `{"message":"stub"}`,
			func(t *testing.T, err error) {
				var bp *apierr.BackpressureError
				if !errors.As(err, &bp) {
					t.Fatalf("want *apierr.BackpressureError, got %T: %v", err, err)
				}
			},
		},
		{
			"429 + different code → BackpressureError", http.StatusTooManyRequests, `{"error":{"code":"too_many_requests","message":"slow down"}}`,
			func(t *testing.T, err error) {
				var bp *apierr.BackpressureError
				if !errors.As(err, &bp) {
					t.Fatalf("want *apierr.BackpressureError, got %T: %v", err, err)
				}
			},
		},
		{
			"429 + quota message (uncoded, flat) → BackpressureError", http.StatusTooManyRequests, `{"error":"Monthly quota exceeded"}`,
			func(t *testing.T, err error) {
				var bp *apierr.BackpressureError
				if !errors.As(err, &bp) {
					t.Fatalf("want *apierr.BackpressureError, got %T: %v", err, err)
				}
			},
		},
		{
			"429 + case-variant code → BackpressureError (exact match only)", http.StatusTooManyRequests, `{"error":{"code":"Tier_Limit_Exceeded"}}`,
			func(t *testing.T, err error) {
				var bp *apierr.BackpressureError
				if !errors.As(err, &bp) {
					t.Fatalf("want *apierr.BackpressureError, got %T: %v", err, err)
				}
			},
		},
		{
			"422 + invalid_environment → plain error (unchanged)", http.StatusUnprocessableEntity, `{"error":{"code":"invalid_environment","message":"bad env"}}`,
			func(t *testing.T, err error) {
				var tl *apierr.TierLimitError
				if errors.As(err, &tl) {
					t.Fatal("invalid_environment must not classify as tier_limit")
				}
				if !strings.Contains(err.Error(), "API error 422") {
					t.Errorf("plain error format changed: %q", err.Error())
				}
			},
		},
		{
			"429 + empty body → BackpressureError", http.StatusTooManyRequests, ``,
			func(t *testing.T, err error) {
				var bp *apierr.BackpressureError
				if !errors.As(err, &bp) {
					t.Fatalf("want *apierr.BackpressureError, got %T: %v", err, err)
				}
			},
		},
		{
			"429 + html body → BackpressureError", http.StatusTooManyRequests, `<html>429</html>`,
			func(t *testing.T, err error) {
				var bp *apierr.BackpressureError
				if !errors.As(err, &bp) {
					t.Fatalf("want *apierr.BackpressureError, got %T: %v", err, err)
				}
			},
		},
		{
			"429 + null error → BackpressureError", http.StatusTooManyRequests, `{"error":null}`,
			func(t *testing.T, err error) {
				var bp *apierr.BackpressureError
				if !errors.As(err, &bp) {
					t.Fatalf("want *apierr.BackpressureError, got %T: %v", err, err)
				}
			},
		},
		{
			"429 + blank code → BackpressureError", http.StatusTooManyRequests, `{"error":{"code":"  "}}`,
			func(t *testing.T, err error) {
				var bp *apierr.BackpressureError
				if !errors.As(err, &bp) {
					t.Fatalf("want *apierr.BackpressureError, got %T: %v", err, err)
				}
			},
		},
		{
			"429 + oversized details → TierLimitError, text capped", http.StatusTooManyRequests,
			`{"error":{"code":"tier_limit_exceeded","message":"cap","details":"` + longDetails + `"}}`,
			func(t *testing.T, err error) {
				var tl *apierr.TierLimitError
				if !errors.As(err, &tl) {
					t.Fatalf("want *apierr.TierLimitError despite >500-byte body, got %T: %v", err, err)
				}
				if len(tl.Details) > maxErrorBodySize+len("... (truncated)") {
					t.Errorf("Details not capped: len=%d", len(tl.Details))
				}
			},
		},
		{
			"503 + coded envelope → ServerError (code ignored outside 4xx)", http.StatusServiceUnavailable, tierLimitEnvelope,
			func(t *testing.T, err error) {
				var se *apierr.ServerError
				if !errors.As(err, &se) {
					t.Fatalf("want *apierr.ServerError, got %T: %v", err, err)
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.assert(t, statusError(tc.status, []byte(tc.body)))
		})
	}
}

// The status-drift learning (docs/solutions/integration-issues/
// gateway-status-code-drift-409-vs-400.md): a typed gate must be proven
// through a real HTTP response, not only by hand-built errors.
func TestClient_TierLimit_RoundTripThroughPost(t *testing.T) {
	for _, status := range []int{http.StatusTooManyRequests, http.StatusForbidden} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(status)
				_, _ = w.Write([]byte(tierLimitEnvelope))
			}))
			defer srv.Close()

			c := New(&config.Config{BaseURL: srv.URL})
			var out map[string]interface{}
			err := c.Post(context.Background(), "/api/v2/keys", map[string]string{"name": "a"}, &out)
			var tl *apierr.TierLimitError
			if !errors.As(err, &tl) {
				t.Fatalf("want *apierr.TierLimitError via Post on %d, got %T: %v", status, err, err)
			}
			if apierr.Kind(err) != apierr.KindTierLimit {
				t.Errorf("Kind = %q, want %q", apierr.Kind(err), apierr.KindTierLimit)
			}
		})
	}
}

// A coded 429 is a hard cap, not backpressure: exactly one attempt.
func TestRetry_TierLimit429_NotRetried(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(tierLimitEnvelope))
	}))
	defer srv.Close()

	attempts, err := runRetry(t, srv, newTestRetryConfig())
	var tl *apierr.TierLimitError
	if !errors.As(err, &tl) {
		t.Fatalf("want *apierr.TierLimitError, got %T: %v", err, err)
	}
	if attempts != 1 {
		t.Errorf("expected 1 attempt for a tier cap, got %d", attempts)
	}
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Errorf("server hit %d times, want 1", got)
	}
}

func TestIsRetryable_TierLimitFalse(t *testing.T) {
	if isRetryable(&apierr.TierLimitError{Status: 429, Code: "tier_limit_exceeded", Message: "cap"}) {
		t.Error("TierLimitError must never be retryable")
	}
}
