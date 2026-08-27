package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Andamio-Platform/andamio-cli/internal/apierr"
	"github.com/Andamio-Platform/andamio-cli/internal/config"
)

const (
	// httpTimeout is the default timeout for HTTP requests
	httpTimeout = 30 * time.Second
	// maxErrorBodySize limits error response body to prevent log flooding
	maxErrorBodySize = 500
)

type Client struct {
	baseURL    string
	apiKey     string
	userJWT    string
	httpClient *http.Client
	// onRetry, when set, is invoked by PostWithRetry between attempts to give
	// the cobra layer a place to log retry progress to stderr without
	// internal/client depending on internal/output. Nil means silent retries.
	onRetry func(attempt int, wait time.Duration, err error)
}

// expiredJWTWarnOnce dedupes the expired-JWT warning to one line per process
// — a single command can construct several clients (tx run, course import).
// It is a resettable package var, not an inline once, so in-package tests can
// reassign it between cases and stay order-independent.
var expiredJWTWarnOnce sync.Once

// New builds a client from cfg. A user-slot JWT that is locally known to be
// expired (config.TokenExpired) is dropped here — the request rides on the
// API key alone — because the gateway fails closed on any invalid credential
// and an expired JWT would otherwise 401 requests the API key alone
// authorizes, including the login-session endpoint itself (issue #134). The
// drop happens on the client's own field snapshot; cfg is never mutated, so
// no later config.Save can persist the cleared slot (R7).
//
// Undecodable tokens are sent as-is (fail open) — the gateway is the
// authority, and a decoder edge case must not lock users out.
//
// Dev-portal surfaces are unaffected by construction order: devKeysClient
// fail-fasts on an expired dev JWT *before* promoting it into the UserJWT
// slot, so a promoted token reaching this drop is known-fresh and the
// user-flavored warning below can only fire on genuine user-slot JWTs.
func New(cfg *config.Config) *Client {
	userJWT := cfg.UserJWT
	if exp, ok := config.TokenExpiry(userJWT); ok && config.ExpiredAt(exp, time.Now()) {
		userJWT = ""
		expiredJWTWarnOnce.Do(func() {
			suffix := "continuing without it."
			if cfg.APIKey != "" {
				suffix = "continuing with API key only."
			}
			// Warnings ride on stderr in every output mode (same judgment as
			// the `dev keys create` one-time-key warning) — scripts pipe
			// 2>/dev/null; humans running --output json still deserve to know
			// their session is dead.
			fmt.Fprintf(os.Stderr, "Warning: stored session expired %s; %s %s\n",
				exp.Local().Format("2006-01-02 15:04 MST"), suffix, cfg.UserJWTRecoveryHint())
		})
	}
	return &Client{
		baseURL:    cfg.BaseURL,
		apiKey:     cfg.APIKey,
		userJWT:    userJWT,
		httpClient: &http.Client{Timeout: httpTimeout},
	}
}

// SetOnRetry registers a callback fired between retry attempts by
// PostWithRetry. The callback runs on the main goroutine; passing nil clears
// the hook. Intended for the cobra layer to emit human-readable "retrying..."
// messages to stderr when not in --output json mode, without the client
// package importing internal/output.
func (c *Client) SetOnRetry(cb func(attempt int, wait time.Duration, err error)) {
	c.onRetry = cb
}

// Get issues a GET request carrying ctx. Cancel ctx to abort the in-flight
// request; passing nil ctx is a programming error and will panic at
// http.NewRequestWithContext.
func (c *Client) Get(ctx context.Context, path string, result interface{}) error {
	url := c.baseURL + path

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return wrapTransportError(ctx, "GET", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return statusError(resp.StatusCode, body)
	}

	return json.NewDecoder(resp.Body).Decode(result)
}

// setHeaders adds common headers to a request. This function emits whatever
// credentials are populated on the client: `X-API-Key` when `apiKey` is set,
// `Authorization: Bearer` when `userJWT` is set — both together if both are
// set. Which credentials end up on the client is the caller's decision, made
// when constructing the cfg passed to `client.New`.
//
// Dual-credential dev-portal surfaces (`/v2/keys`, `/api/v2/apikey/developer/*`,
// and other developer-portal endpoints) require BOTH `X-API-Key` (gateway
// `V2AuthMiddleware`) AND `Authorization: Bearer <devJWT>` (`developerJWTAuth`).
// Routing for these is handled by `cmd/andamio/dev_keys.go`'s `devKeysClient`,
// which clones the cfg, **preserves** `APIKey`, and promotes `DevJWT` into the
// `UserJWT` slot so both headers ride on the request (the wallet/user JWT is
// overwritten, not appended). `cmd/andamio/apikey.go` is a second consumer of
// that helper. Background:
// `docs/solutions/integration-issues/cli-apikey-auth-isolation-and-content-404-ux.md`
// (Issue #17 portion superseded — the gateway flipped from rejecting to
// requiring the dev JWT on this surface). Before changing how credentials are
// selected for a dev-portal surface, weigh the dual-credential failure modes
// the gateway middleware checks for.
func (c *Client) setHeaders(req *http.Request) {
	if c.apiKey != "" {
		req.Header.Set("X-API-Key", c.apiKey)
	}
	if c.userJWT != "" {
		req.Header.Set("Authorization", "Bearer "+c.userJWT)
	}
	req.Header.Set("Accept", "application/json")
}

// Post sends a POST request with JSON body and decodes the response. See Get
// for ctx semantics.
func (c *Client) Post(ctx context.Context, path string, body interface{}, result interface{}) error {
	url := c.baseURL + path

	var reqBody io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reqBody = bytes.NewReader(jsonData)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, reqBody)
	if err != nil {
		return err
	}

	c.setHeaders(req)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return wrapTransportError(ctx, "POST", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return statusError(resp.StatusCode, respBody)
	}

	if result != nil {
		return json.NewDecoder(resp.Body).Decode(result)
	}
	return nil
}

// Put sends a PUT request with JSON body and decodes the response. See Get
// for ctx semantics.
func (c *Client) Put(ctx context.Context, path string, body interface{}, result interface{}) error {
	url := c.baseURL + path

	var reqBody io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reqBody = bytes.NewReader(jsonData)
	}

	req, err := http.NewRequestWithContext(ctx, "PUT", url, reqBody)
	if err != nil {
		return err
	}

	c.setHeaders(req)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return wrapTransportError(ctx, "PUT", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return statusError(resp.StatusCode, respBody)
	}

	if result != nil {
		return json.NewDecoder(resp.Body).Decode(result)
	}
	return nil
}

// Delete sends a DELETE request and returns nil on any 2xx. The result
// param mirrors Post/Put: pass nil for endpoints that return 204 No Content
// (the common case — DELETE /v2/keys/{id} is one), pass a struct pointer
// for endpoints that return 200 with a body. See Get for ctx semantics.
func (c *Client) Delete(ctx context.Context, path string, result interface{}) error {
	url := c.baseURL + path

	req, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
	if err != nil {
		return err
	}

	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return wrapTransportError(ctx, "DELETE", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return statusError(resp.StatusCode, body)
	}
	if result != nil {
		return json.NewDecoder(resp.Body).Decode(result)
	}
	return nil
}

// wrapTransportError classifies an error from http.Client.Do.
//
// Context cancellation and deadline expiry are returned unchanged. An operator
// pressing Ctrl-C, a --timeout expiring, and a service being unreachable are
// three different outcomes; wrapping the first two as NetworkError would tell a
// caller the network is down when the caller is the one who stopped. Everything
// else that prevented the request from completing — DNS failure, connection
// refused, TLS failure, a connection dropped mid-flight — is genuinely "could
// not reach the service".
//
// Note this classifies transport failures only. A response that arrives and
// carries an error status goes through statusError instead.
func wrapTransportError(ctx context.Context, method, url string, err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	// A cancelled context can surface as an unrelated transport error depending
	// on where in the request lifecycle the cancellation landed, so consult the
	// context itself rather than trusting the error alone.
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	return &apierr.NetworkError{
		Message: fmt.Sprintf("could not reach %s %s: %v", method, url, err),
		Err:     err,
	}
}

// gatewayCodeTierLimitExceeded is andamio-api's stable body code for "the
// account's plan does not permit this" (keys_viewmodels.ErrCodeTierLimitExceeded).
// The CLI exit-code contract consumes it verbatim: renaming it gateway-side
// silently disables the tier_limit classification and needs a coordinated CLI
// release.
const gatewayCodeTierLimitExceeded = "tier_limit_exceeded"

// decodeGatewayErrorCode tolerantly reads the gateway's error envelope. The
// current shape is {"error":{"code":"…","message":"…","details":"…"}}; some
// routes still emit the flat {"error":"…"} form, in which case the string is
// returned as the code with no message. Returns ok=false for an empty or
// non-JSON body, a null or missing error field, or a blank code — every miss
// falls through to the status switch, so nothing currently classified changes
// unless a code matches exactly.
func decodeGatewayErrorCode(body []byte) (code, message, details string, ok bool) {
	if len(bytes.TrimSpace(body)) == 0 {
		return "", "", "", false
	}
	var envelope struct {
		Error json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return "", "", "", false
	}
	raw := bytes.TrimSpace(envelope.Error)
	if len(raw) == 0 || string(raw) == "null" {
		return "", "", "", false
	}
	switch raw[0] {
	case '{':
		var detail struct {
			Code    string `json:"code"`
			Message string `json:"message"`
			Details string `json:"details"`
		}
		if err := json.Unmarshal(raw, &detail); err != nil {
			return "", "", "", false
		}
		code = strings.TrimSpace(detail.Code)
		message, details = strings.TrimSpace(detail.Message), strings.TrimSpace(detail.Details)
	case '"':
		var flat string
		if err := json.Unmarshal(raw, &flat); err != nil {
			return "", "", "", false
		}
		code = strings.TrimSpace(flat)
	default:
		return "", "", "", false
	}
	if code == "" {
		return "", "", "", false
	}
	return code, message, details, true
}

// statusError maps an HTTP error status to the typed error the CLI expects.
// A 4xx whose body code is tier_limit_exceeded → TierLimitError (checked first);
// 401/403 → AuthError, 404 → NotFoundError, 409 → ConflictError,
// 408/425/429 → BackpressureError (retryable transient backpressure),
// 5xx → ServerError, anything else → plain error. Error message format
// ("API error %d: %s") is preserved across all branches so downstream
// string-match consumers (if any) keep working.
func statusError(status int, body []byte) error {
	// Plan-gated refusals are classified by the gateway's body code, not by
	// status, and before the status switch: a tier cap arrives on 429 today
	// and is ruled to move to 403 (product-circle#304). Keying on the code
	// means neither status can misfile it as backpressure (retried) or auth
	// (re-login). Only 4xx bodies are consulted — a 5xx carrying the code is
	// still a server failure. Decoded from the raw body (the gateway
	// pretty-prints; truncating first could cut the closing braces), then
	// capped, so the 500-byte bound still holds for what reaches the user.
	if status >= 400 && status < 500 {
		if code, message, details, ok := decodeGatewayErrorCode(body); ok && code == gatewayCodeTierLimitExceeded {
			if message == "" {
				message = truncateErrorBody(body)
			}
			return &apierr.TierLimitError{
				Status:  status,
				Code:    code,
				Message: truncateErrorBody([]byte(message)),
				Details: truncateErrorBody([]byte(details)),
			}
		}
	}

	msg := fmt.Sprintf("API error %d: %s", status, truncateErrorBody(body))
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return &apierr.AuthError{HTTPStatus: status, Message: msg}
	case http.StatusNotFound:
		return &apierr.NotFoundError{Message: msg}
	case http.StatusConflict:
		return &apierr.ConflictError{Message: msg}
	case http.StatusRequestTimeout, http.StatusTooEarly, http.StatusTooManyRequests:
		return &apierr.BackpressureError{
			Status:            status,
			Message:           msg,
			RetryAfterSeconds: parseRetryAfterSeconds(body),
		}
	}
	if status >= 500 && status < 600 {
		return &apierr.ServerError{Status: status, Message: msg}
	}
	return errors.New(msg)
}

// parseRetryAfterSeconds tolerantly parses a "Retry-After: N" hint from a
// response body. Only integer seconds are accepted. Returns 0 on any parse
// failure so callers can fall through to exponential backoff. This is a body
// parse, not a header parse — the CLI currently does not surface HTTP headers
// at the client boundary.
func parseRetryAfterSeconds(body []byte) int {
	s := string(body)
	const key = "Retry-After:"
	idx := -1
	for i := 0; i+len(key) <= len(s); i++ {
		if s[i:i+len(key)] == key {
			idx = i + len(key)
			break
		}
	}
	if idx < 0 {
		return 0
	}
	for idx < len(s) && (s[idx] == ' ' || s[idx] == '\t') {
		idx++
	}
	end := idx
	for end < len(s) && s[end] >= '0' && s[end] <= '9' {
		end++
	}
	if end == idx {
		return 0
	}
	n := 0
	for i := idx; i < end; i++ {
		n = n*10 + int(s[i]-'0')
		if n < 0 || n > 1<<30 {
			// Overflow or unreasonable — bail to avoid wild sleeps.
			return 0
		}
	}
	return n
}

// truncateErrorBody limits error message size to prevent log flooding and info leakage
func truncateErrorBody(body []byte) string {
	s := string(body)
	if len(s) > maxErrorBodySize {
		return s[:maxErrorBodySize] + "... (truncated)"
	}
	return s
}
