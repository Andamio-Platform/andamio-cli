package client

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestDecodeGatewayError(t *testing.T) {
	cases := []struct {
		name   string
		body   string
		wantOK bool
		want   gatewayError
	}{
		{"nested envelope", tierLimitEnvelope, true, gatewayError{"tier_limit_exceeded", "maximum API key limit (1) reached for your tier; revoke an existing key or upgrade your subscription", ""}},
		{"nested with details", `{"error":{"code":"x","message":"m","details":"d"}}`, true, gatewayError{"x", "m", "d"}},
		{"flat string error", `{"error":"tier_limit_exceeded"}`, true, gatewayError{"tier_limit_exceeded", "", ""}},
		{"flat legacy with message sibling", `{"error":"tier_limit_exceeded","message":"tier cap reached"}`, true, gatewayError{"tier_limit_exceeded", "tier cap reached", ""}},
		{"nested ignores top-level siblings", `{"error":{"code":"x","message":"inner"},"message":"outer"}`, true, gatewayError{"x", "inner", ""}},
		{"whitespace trimmed", `{"error":{"code":"  x  ","message":" m "}}`, true, gatewayError{"x", "m", ""}},
		{"empty body", ``, false, gatewayError{}},
		{"whitespace body", "  \n ", false, gatewayError{}},
		{"html body", `<html><body>502 Bad Gateway</body></html>`, false, gatewayError{}},
		{"plain text body", "backpressure\n", false, gatewayError{}},
		{"no error field", `{"message":"stub"}`, false, gatewayError{}},
		{"null error", `{"error":null}`, false, gatewayError{}},
		{"blank code", `{"error":{"code":"   ","message":"m"}}`, false, gatewayError{}},
		{"missing code", `{"error":{"message":"m"}}`, false, gatewayError{}},
		{"array error", `{"error":["x"]}`, false, gatewayError{}},
		{"numeric error", `{"error":42}`, false, gatewayError{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := decodeGatewayError([]byte(tc.body))
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v (got %+v)", ok, tc.wantOK, got)
			}
			if ok && got != tc.want {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

// Classification keys on the body code, on any 4xx, before the status switch.
// Every other body on every other status must land exactly where it did
// before — each tier_limit row is paired with the untouched sibling.
func TestStatusError_TierLimitByBodyCode(t *testing.T) {
	cases := []struct {
		name     string
		status   int
		body     string
		wantKind string
	}{
		{"429 + coded envelope", 429, tierLimitEnvelope, apierr.KindTierLimit},
		{"403 + coded envelope (product-circle#304 forward-compat)", 403, tierLimitEnvelope, apierr.KindTierLimit},
		{"429 + flat coded body", 429, `{"error":"tier_limit_exceeded"}`, apierr.KindTierLimit},
		{"429 + {message:stub}", 429, `{"message":"stub"}`, apierr.KindBackpressure},
		{"429 + different code", 429, `{"error":{"code":"too_many_requests","message":"slow down"}}`, apierr.KindBackpressure},
		{"429 + uncoded quota message", 429, `{"error":"Monthly quota exceeded"}`, apierr.KindBackpressure},
		{"429 + case-variant code (exact match only)", 429, `{"error":{"code":"Tier_Limit_Exceeded"}}`, apierr.KindBackpressure},
		{"429 + code only mentioned in message text", 429, `{"error":{"code":"other","message":"see tier_limit_exceeded"}}`, apierr.KindBackpressure},
		{"422 + invalid_environment", 422, `{"error":{"code":"invalid_environment","message":"bad env"}}`, apierr.KindError},
		{"429 + empty body", 429, ``, apierr.KindBackpressure},
		{"429 + html body", 429, `<html>429</html>`, apierr.KindBackpressure},
		{"429 + null error", 429, `{"error":null}`, apierr.KindBackpressure},
		{"429 + blank code", 429, `{"error":{"code":"  "}}`, apierr.KindBackpressure},
		{"503 + coded envelope (code ignored outside 4xx)", 503, tierLimitEnvelope, apierr.KindServer},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := statusError(tc.status, []byte(tc.body))
			if got := apierr.Kind(err); got != tc.wantKind {
				t.Errorf("Kind = %q, want %q (%T: %v)", got, tc.wantKind, err, err)
			}
		})
	}
}

func TestStatusError_TierLimitFields(t *testing.T) {
	var tl *apierr.TierLimitError
	if err := statusError(429, []byte(tierLimitEnvelope)); !errors.As(err, &tl) {
		t.Fatalf("want *apierr.TierLimitError, got %T", err)
	}
	if tl.Status != 429 || tl.Code != "tier_limit_exceeded" {
		t.Errorf("Status/Code = %d/%q", tl.Status, tl.Code)
	}
	if !strings.Contains(tl.Message, "revoke an existing key or upgrade") {
		t.Errorf("Message should carry the gateway sentence verbatim, got %q", tl.Message)
	}
	if !strings.Contains(tl.Error(), "tier_limit_exceeded") {
		t.Errorf("Error() must keep the stable code string, got %q", tl.Error())
	}

	// Flat shape carries no message: fall back to the body so the user still
	// sees something.
	if err := statusError(429, []byte(`{"error":"tier_limit_exceeded"}`)); !errors.As(err, &tl) || tl.Message == "" {
		t.Errorf("flat-shape Message should fall back to the body, got %v", err)
	}

	// Oversized details: still classified (decode is on the raw body), text
	// still capped.
	long := `{"error":{"code":"tier_limit_exceeded","message":"cap","details":"` + strings.Repeat("d", 700) + `"}}`
	if err := statusError(429, []byte(long)); !errors.As(err, &tl) {
		t.Fatalf("want *apierr.TierLimitError despite >500-byte body, got %T", err)
	} else if len(tl.Details) > maxErrorBodySize+len("... (truncated)") {
		t.Errorf("Details not capped: len=%d", len(tl.Details))
	}
}

// The status-drift learning (docs/solutions/integration-issues/
// gateway-status-code-drift-409-vs-400.md): a typed gate must be proven
// through a real HTTP response, not only by hand-built errors. Driving it
// through the retry helper also proves a coded 429 makes exactly one attempt.
func TestClient_TierLimit_RoundTripNotRetried(t *testing.T) {
	for _, status := range []int{http.StatusTooManyRequests, http.StatusForbidden} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(status)
				_, _ = w.Write([]byte(tierLimitEnvelope))
			}))
			defer srv.Close()

			attempts, err := runRetry(t, srv, newTestRetryConfig())
			if got := apierr.Kind(err); got != apierr.KindTierLimit {
				t.Fatalf("Kind = %q, want %q via Post on %d (%T: %v)", got, apierr.KindTierLimit, status, err, err)
			}
			if attempts != 1 {
				t.Errorf("expected 1 attempt for a tier cap, got %d", attempts)
			}

			c := New(&config.Config{BaseURL: srv.URL})
			var out map[string]interface{}
			var tl *apierr.TierLimitError
			if err := c.Post(context.Background(), "/api/v2/keys", map[string]string{"name": "a"}, &out); !errors.As(err, &tl) {
				t.Errorf("plain Post: want *apierr.TierLimitError, got %T: %v", err, err)
			}
		})
	}
}
