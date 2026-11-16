package submit

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const submitTimeout = 60 * time.Second

// SubmitTransaction sends a signed transaction (CBOR hex) to a Cardano submit API.
// Returns the response body on success. Custom headers (e.g., "project_id: abc") can be
// passed for provider authentication.
func SubmitTransaction(submitURL, signedCBORHex string, headers []string) ([]byte, error) {
	cborBytes, err := hex.DecodeString(signedCBORHex)
	if err != nil {
		return nil, fmt.Errorf("invalid CBOR hex: %w", err)
	}

	req, err := http.NewRequest("POST", submitURL, bytes.NewReader(cborBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	for _, h := range headers {
		parts := strings.SplitN(h, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid header format %q: expected \"Key: Value\"", h)
		}
		req.Header.Set(strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]))
	}

	// Set Content-Type after user headers to prevent accidental override.
	// The submit API requires application/cbor — overriding this breaks submission.
	req.Header.Set("Content-Type", "application/cbor")

	// Redirects are not followed. Go's client would follow a 301/302 as a GET,
	// dropping the transaction, and a 200 from the redirect target (Blockfrost
	// sends a wrong path to its homepage) would then pass for a successful
	// submit. A redirect is reported as an error below instead.
	client := &http.Client{
		Timeout: submitTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s: %w", submitURL, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1 MB limit
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		return nil, fmt.Errorf("submit API redirected (%d to %q), so the transaction was not submitted; check the submit URL (Blockfrost's ends in /api/v0/tx/submit)", resp.StatusCode, resp.Header.Get("Location"))
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("submit API error %d: %s", resp.StatusCode, truncate(string(body), 500))
	}

	return body, nil
}

func truncate(s string, max int) string {
	if len(s) > max {
		return s[:max] + "... (truncated)"
	}
	return s
}
