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

	req.Header.Set("Content-Type", "application/cbor")

	for _, h := range headers {
		parts := strings.SplitN(h, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid header format %q: expected \"Key: Value\"", h)
		}
		req.Header.Set(strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]))
	}

	client := &http.Client{Timeout: submitTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s: %w", submitURL, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1 MB limit
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
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
