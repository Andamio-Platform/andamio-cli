package client

import (
	"bytes"
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

func (c *Client) Get(path string, result interface{}) error {
	url := c.baseURL + path

	req, err := http.NewRequest("GET", url, nil)
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
		msg := fmt.Sprintf("API error %d: %s", resp.StatusCode, truncateErrorBody(body))
		switch resp.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return &apierr.AuthError{Message: msg}
		case http.StatusNotFound:
			return &apierr.NotFoundError{Message: msg}
		case http.StatusConflict:
			return &apierr.ConflictError{Message: msg}
		}
		return errors.New(msg)
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

// Post sends a POST request with JSON body and decodes the response.
func (c *Client) Post(path string, body interface{}, result interface{}) error {
	url := c.baseURL + path

	var reqBody io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reqBody = bytes.NewReader(jsonData)
	}

	req, err := http.NewRequest("POST", url, reqBody)
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
		msg := fmt.Sprintf("API error %d: %s", resp.StatusCode, truncateErrorBody(respBody))
		switch resp.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return &apierr.AuthError{Message: msg}
		case http.StatusNotFound:
			return &apierr.NotFoundError{Message: msg}
		case http.StatusConflict:
			return &apierr.ConflictError{Message: msg}
		}
		return errors.New(msg)
	}

	if result != nil {
		return json.NewDecoder(resp.Body).Decode(result)
	}
	return nil
}

// Put sends a PUT request with JSON body and decodes the response.
func (c *Client) Put(path string, body interface{}, result interface{}) error {
	url := c.baseURL + path

	var reqBody io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reqBody = bytes.NewReader(jsonData)
	}

	req, err := http.NewRequest("PUT", url, reqBody)
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
		msg := fmt.Sprintf("API error %d: %s", resp.StatusCode, truncateErrorBody(respBody))
		switch resp.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return &apierr.AuthError{Message: msg}
		case http.StatusNotFound:
			return &apierr.NotFoundError{Message: msg}
		case http.StatusConflict:
			return &apierr.ConflictError{Message: msg}
		}
		return errors.New(msg)
	}

	if result != nil {
		return json.NewDecoder(resp.Body).Decode(result)
	}
	return nil
}

// truncateErrorBody limits error message size to prevent log flooding and info leakage
func truncateErrorBody(body []byte) string {
	s := string(body)
	if len(s) > maxErrorBodySize {
		return s[:maxErrorBodySize] + "... (truncated)"
	}
	return s
}
