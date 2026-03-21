package main

import (
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"unicode/utf8"

	"github.com/Andamio-Platform/andamio-cli/internal/apierr"
	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/Andamio-Platform/andamio-cli/internal/config"
	"github.com/Andamio-Platform/andamio-cli/internal/output"
	"github.com/spf13/cobra"
)

// jwtAuthPreRunE is a shared PersistentPreRunE that chains with root (for --output flag)
// and checks for JWT authentication. Used by all role-based parent commands.
func jwtAuthPreRunE(cmd *cobra.Command, args []string) error {
	if err := rootCmd.PersistentPreRunE(cmd, args); err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if !cfg.HasUserAuth() {
		return &apierr.AuthError{Message: "not authenticated. Run 'andamio user login' first"}
	}
	return nil
}

// getJSON is a helper for simple GET endpoints that return JSON
func getJSON(path string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	c := client.New(cfg)
	var result map[string]interface{}
	if err := c.Get(path, &result); err != nil {
		return err
	}

	return output.PrintJSON(result)
}

// postJSON is a helper for simple POST endpoints that return JSON (no body)
func postJSON(path string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	c := client.New(cfg)
	var result map[string]interface{}
	if err := c.Post(path, nil, &result); err != nil {
		return err
	}

	return output.PrintJSON(result)
}

// getJSONWithHint wraps getJSON and replaces NotFoundError messages with a contextual hint.
func getJSONWithHint(path, notFoundHint string) error {
	err := getJSON(path)
	if err != nil {
		var notFound *apierr.NotFoundError
		if errors.As(err, &notFound) {
			return &apierr.NotFoundError{Message: notFoundHint}
		}
		return err
	}
	return nil
}

// truncateUTF8 truncates a string to maxRunes runes, appending "..." if truncated.
func truncateUTF8(s string, maxRunes int) string {
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxRunes-3]) + "..."
}

// printList fetches a list endpoint and prints using PrintList
func printList(path, emptyMsg, titleKey, idKey string, usePost bool) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	c := client.New(cfg)
	var response map[string]interface{}
	var reqErr error
	if usePost {
		reqErr = c.Post(path, nil, &response)
	} else {
		reqErr = c.Get(path, &response)
	}
	if reqErr != nil {
		return reqErr
	}

	data, ok := response["data"].([]interface{})
	if !ok || len(data) == 0 {
		if output.GetFormat() == output.FormatJSON {
			return output.PrintJSON(map[string]interface{}{"data": []interface{}{}})
		} else {
			fmt.Fprintln(os.Stderr, emptyMsg)
		}
		return nil
	}

	items := make([]map[string]interface{}, 0, len(data))
	for _, item := range data {
		if m, ok := item.(map[string]interface{}); ok {
			items = append(items, m)
		}
	}

	return output.PrintList(items, titleKey, idKey)
}

// printListPost fetches a POST list endpoint with a payload and prints using PrintList.
// Use this for role-based list endpoints that require a body (e.g., project-id filter).
func printListPost(path string, payload interface{}, emptyMsg, titleKey, idKey string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	c := client.New(cfg)
	var response map[string]interface{}
	if err := c.Post(path, payload, &response); err != nil {
		return err
	}

	if output.GetFormat() == output.FormatJSON {
		return output.PrintJSON(response)
	}

	data, ok := response["data"].([]interface{})
	if !ok || len(data) == 0 {
		fmt.Fprintln(os.Stderr, emptyMsg)
		return nil
	}

	items := make([]map[string]interface{}, 0, len(data))
	for _, item := range data {
		if m, ok := item.(map[string]interface{}); ok {
			items = append(items, m)
		}
	}

	return output.PrintList(items, titleKey, idKey)
}

// isHex returns true if s is a valid hex-encoded string (even length, all hex chars).
func isHex(s string) bool {
	if len(s) == 0 || len(s)%2 != 0 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

// hexEncodeAssetName hex-encodes an asset name if it is not already hex.
// Empty strings are passed through unchanged.
func hexEncodeAssetName(name string) string {
	if name == "" || isHex(name) {
		return name
	}
	return hex.EncodeToString([]byte(name))
}

// hexDecodeAssetName attempts to decode a hex-encoded asset name to UTF-8.
// Returns the original string if decoding fails or produces non-UTF-8.
func hexDecodeAssetName(name string) string {
	if name == "" {
		return name
	}
	decoded, err := hex.DecodeString(name)
	if err != nil {
		return name
	}
	s := string(decoded)
	if !utf8.ValidString(s) {
		return name
	}
	return s
}
