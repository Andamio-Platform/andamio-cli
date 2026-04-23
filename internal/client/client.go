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
// 5xx → ServerError, anything else → plain error. Error message format
// ("API error %d: %s") is preserved across all branches.
func statusError(status int, body []byte) error {
	msg := fmt.Sprintf("API error %d: %s", status, truncateErrorBody(body))
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return &apierr.AuthError{Message: msg}
	case http.StatusNotFound:
		return &apierr.NotFoundError{Message: msg}
	case http.StatusConflict:
		return &apierr.ConflictError{Message: msg}
	}
	if status >= 500 && status < 600 {
		return &apierr.ServerError{Status: status, Message: msg}
	}
	return errors.New(msg)
}

// truncateErrorBody limits error message size to prevent log flooding and info leakage
func truncateErrorBody(body []byte) string {
	s := string(body)
	if len(s) > maxErrorBodySize {
		return s[:maxErrorBodySize] + "... (truncated)"
	}
	return s
}
