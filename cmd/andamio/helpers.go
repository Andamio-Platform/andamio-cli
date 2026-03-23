package main

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/Andamio-Platform/andamio-cli/internal/apierr"
	"github.com/Andamio-Platform/andamio-cli/internal/cardano"
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

// normalizeForHashing normalizes a value for deterministic hashing.
// Primary purpose: trims whitespace from strings to match @andamio/core
// computeCommitmentHash. Go's json.Marshal already sorts map keys alphabetically;
// this function adds string trimming and recursive normalization.
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

// wrapEvidence converts evidence text to a Tiptap JSON document and computes its Blake2b-256 content hash.
// The hash matches @andamio/core computeCommitmentHash: normalize → JSON.stringify → Blake2b-256.
// Returns the Tiptap document as a map (for embedding as a JSON object in payloads) and the hex hash.
func wrapEvidence(text string) (map[string]interface{}, string, error) {
	tiptapDoc, err := markdownToTiptap(text, nil)
	if err != nil {
		return nil, "", fmt.Errorf("markdown to tiptap: %w", err)
	}

	// Normalize to match frontend: sort keys, trim strings
	normalized := normalizeForHashing(tiptapDoc)

	jsonBytes, err := json.Marshal(normalized)
	if err != nil {
		return nil, "", fmt.Errorf("json marshal: %w", err)
	}

	hash := cardano.Blake2b256(jsonBytes)
	// Return the normalized doc (what the hash was computed from)
	normalizedDoc, _ := normalized.(map[string]interface{})
	return normalizedDoc, hex.EncodeToString(hash), nil
}

// resolveTaskHash looks up the task_hash for a given project + task index.
// Fetches the user-visible task list and matches by task_index.
func resolveTaskHash(c *client.Client, projectID string, taskIndex int) (string, error) {
	body := map[string]string{"project_id": projectID}
	var resp map[string]interface{}
	if err := c.Post("/api/v2/project/user/tasks/list", body, &resp); err != nil {
		return "", fmt.Errorf("failed to list tasks: %w", err)
	}

	data, ok := resp["data"].([]interface{})
	if !ok {
		return "", fmt.Errorf("no tasks found for project %s", projectID)
	}

	for _, item := range data {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		idx, _ := m["task_index"].(float64)
		if int(idx) == taskIndex {
			hash, _ := m["task_hash"].(string)
			if hash == "" {
				return "", fmt.Errorf("task %d has no task_hash (may not be on-chain yet)", taskIndex)
			}
			return hash, nil
		}
	}
	return "", fmt.Errorf("task index %d not found in project %s\n\nList tasks with:\n  andamio project tasks %s --output json", taskIndex, projectID, projectID)
}

// resolveSltHash looks up the slt_hash for a given course + module code.
// Fetches the course modules list and matches by module code.
func resolveSltHash(c *client.Client, courseID, moduleCode string) (string, error) {
	path := "/api/v2/course/user/modules/" + url.PathEscape(courseID)
	var resp map[string]interface{}
	if err := c.Get(path, &resp); err != nil {
		return "", fmt.Errorf("failed to list modules: %w", err)
	}

	data, ok := resp["data"].([]interface{})
	if !ok {
		return "", fmt.Errorf("no modules found for course %s", courseID)
	}

	for _, item := range data {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		code, _ := m["course_module_code"].(string)
		if code == moduleCode {
			hash, _ := m["slt_hash"].(string)
			if hash == "" {
				return "", fmt.Errorf("module %s has no slt_hash (may not be on-chain yet)", moduleCode)
			}
			return hash, nil
		}
	}
	return "", fmt.Errorf("module %s not found in course %s\n\nList modules with:\n  andamio course modules %s --output json", moduleCode, courseID, courseID)
}

// readEvidenceFlag reads the evidence text from either --evidence or --evidence-file.
// The two flags are mutually exclusive; at least one must be set.
func readEvidenceFlag(cmd *cobra.Command) (string, error) {
	evidence, _ := cmd.Flags().GetString("evidence")
	evidenceFile, _ := cmd.Flags().GetString("evidence-file")

	if evidence != "" && evidenceFile != "" {
		return "", fmt.Errorf("--evidence and --evidence-file are mutually exclusive")
	}

	if evidenceFile != "" {
		data, err := os.ReadFile(evidenceFile)
		if err != nil {
			return "", fmt.Errorf("failed to read evidence file %s: %w", evidenceFile, err)
		}
		evidence = strings.TrimSpace(string(data))
	}

	if evidence == "" {
		return "", fmt.Errorf("evidence is required: use --evidence or --evidence-file")
	}
	return evidence, nil
}
