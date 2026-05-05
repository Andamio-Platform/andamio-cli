package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
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

func New(cfg *config.Config) *Client {
	return &Client{
		baseURL:    cfg.BaseURL,
		apiKey:     cfg.APIKey,
		userJWT:    cfg.UserJWT,
		httpClient: &http.Client{Timeout: httpTimeout},
	}
}

// SetUserJWT sets the user JWT for authenticated requests.
func (c *Client) SetUserJWT(jwt string) {
	c.userJWT = jwt
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
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return statusError(resp.StatusCode, body)
	}

	return json.NewDecoder(resp.Body).Decode(result)
}

// setHeaders adds common headers to a request.
//
// TODO(#80 PR-B): wire developer JWT (`cfg.DevJWT`) into the Authorization
// header for `/v2/keys` and other developer-portal endpoints. The gateway's
// `developerJWTAuth` middleware does not accept wallet/user JWTs; the dev
// JWT is a separate credential minted by `andamio dev login` and lives in a
// distinct config slot. Today this client only forwards `userJWT`. PR-A
// (issue #80, this branch) ships the login + storage; PR-B follows with
// either (a) a `Client.SetDevJWT` builder + per-call swap, or (b) a clone
// of the cfg with `DevJWT` promoted into `UserJWT` for keys requests
// (mirroring the api-key-only swap in `cmd/andamio/apikey.go`). Until PR-B
// lands, callers of `/v2/keys` have no auth path through this client.
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
		return err
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
		return err
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

// statusError maps an HTTP error status to the typed error the CLI expects.
// 401/403 → AuthError, 404 → NotFoundError, 409 → ConflictError,
// 408/425/429 → BackpressureError (retryable transient backpressure),
// 5xx → ServerError, anything else → plain error. Error message format
// ("API error %d: %s") is preserved across all branches so downstream
// string-match consumers (if any) keep working.
func statusError(status int, body []byte) error {
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
