package main

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
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
func getJSON(ctx context.Context, path string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	c := client.New(cfg)
	var result map[string]interface{}
	if err := c.Get(ctx, path, &result); err != nil {
		return err
	}

	return output.PrintJSON(result)
}

// postJSON is a helper for simple POST endpoints that return JSON (no body)
func postJSON(ctx context.Context, path string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	c := client.New(cfg)
	var result map[string]interface{}
	if err := c.Post(ctx, path, nil, &result); err != nil {
		return err
	}

	return output.PrintJSON(result)
}

// getJSONWithHint wraps getJSON and replaces NotFoundError messages with a contextual hint.
func getJSONWithHint(ctx context.Context, path, notFoundHint string) error {
	err := getJSON(ctx, path)
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

// padRunes right-pads s with spaces to width runes (not bytes). Go's fmt width
// specifier counts bytes, so multi-byte content like "—" (3 bytes, 1 rune)
// would underpad visible columns with %-Ns. Use this when a dynamic-width
// column may contain non-ASCII content and the next column must stay aligned.
func padRunes(s string, width int) string {
	n := utf8.RuneCountInString(s)
	if n >= width {
		return s
	}
	return s + strings.Repeat(" ", width-n)
}

// printList fetches a list endpoint and prints using PrintList.
//
// An empty result is a success, not an error: JSON mode emits {"data": []} and
// the command exits 0. That is deliberate and load-bearing. "Nothing found",
// "could not reach the service" and "not permitted" are three different
// outcomes that a caller has to act on differently, and collapsing the first
// into an error would put it back on the same footing as the other two.
// Exit codes 0 / 5 / 3 and kinds ""/unreachable/auth keep them apart — see the
// exit-code contract in main.go.
//
// The human-readable empty message goes to stderr so stdout stays parseable.
func printList(ctx context.Context, path, emptyMsg, titleKey, idKey string, usePost bool) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	c := client.New(cfg)
	var response map[string]interface{}
	var reqErr error
	if usePost {
		reqErr = c.Post(ctx, path, nil, &response)
	} else {
		reqErr = c.Get(ctx, path, &response)
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
func printListPost(ctx context.Context, path string, payload interface{}, emptyMsg, titleKey, idKey string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	c := client.New(cfg)
	var response map[string]interface{}
	if err := c.Post(ctx, path, payload, &response); err != nil {
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

// normalizeForHashing normalizes a value for deterministic hashing.
// Primary purpose: trims whitespace from strings to match @andamio/core
// computeCommitmentHash. Go's json.Marshal already sorts map keys alphabetically;
// this function adds string trimming and recursive normalization.
//
// NOT DEAD CODE, despite having no production caller since 1.0. Evidence
// submission moved to the Andamio app when the learner and contributor surface
// was removed (issue #129), which retired this function's only command-side
// consumer. It is deliberately retained because
// commitment_hash_parity_test.go pins it against a hand-verified
// @andamio/core vector — a cross-repo contract that took real work to
// establish and that would have to be rebuilt from scratch if evidence
// hashing returns to the CLI (learner support is scoped out of 1.0, not ruled
// out). Deleting this means deleting that guarantee; do it deliberately or
// not at all.
func normalizeForHashing(v interface{}) interface{} {
	if v == nil {
		return nil
	}
	switch val := v.(type) {
	case string:
		return strings.TrimSpace(val)
	case map[string]interface{}:
		sorted := make(map[string]interface{}, len(val))
		for k, child := range val {
			sorted[k] = normalizeForHashing(child)
		}
		return sorted
	case []interface{}:
		out := make([]interface{}, len(val))
		for i, child := range val {
			out[i] = normalizeForHashing(child)
		}
		return out
	default:
		return v // numbers, booleans
	}
}
