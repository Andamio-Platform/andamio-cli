package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/Andamio-Platform/andamio-cli/internal/config"
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
		httpClient: &http.Client{},
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
		return fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
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
		return fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
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
		return fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
	}

	if result != nil {
		return json.NewDecoder(resp.Body).Decode(result)
	}
	return nil
}
