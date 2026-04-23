package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Andamio-Platform/andamio-cli/internal/cardano"
	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/Andamio-Platform/andamio-cli/internal/config"
	"github.com/Andamio-Platform/andamio-cli/internal/output"
	"github.com/adrg/frontmatter"
	"github.com/spf13/cobra"
)

var projectTaskCmd = &cobra.Command{
	Use:               "task",
	Short:             "Manage project tasks (manager role)",
	PersistentPreRunE: jwtAuthPreRunE,
}

var projectTaskListCmd = &cobra.Command{
	Use:   "list <project-id>",
	Short: "List tasks for a project",
	Long: `List tasks for a project where you are a manager.

Find your project IDs with: andamio project list --output json

Requires user authentication via 'andamio user login'.`,
	Args: cobra.ExactArgs(1),
	RunE: runTasksList,
}

var projectTaskCreateCmd = &cobra.Command{
	Use:   "create <project-id>",
	Short: "Create a new task",
	Long: `Create a new task for a project where you are a manager.

Find your project IDs with: andamio project list --output json

Examples:
  andamio project task create <project-id> --title "Build API" --lovelace 5000000 --expiration 2026-04-01T00:00:00Z
  andamio project task create <project-id> --title "Fix bug" --lovelace 2000000 --expiration 2026-04-01T00:00:00Z --github-issue "org/repo#42"
  andamio project task create <project-id> --title "Design system" --lovelace 5000000 --expiration 2026-04-01 --content-file task.md
  andamio project task create <project-id> --title "Earn XP" --lovelace 5000000 --expiration 2026-04-01 --token "policyid...,XP,50"

Requires user authentication via 'andamio user login'.`,
	Args: cobra.ExactArgs(1),
	RunE: runTaskCreate,
}

var projectTaskGetCmd = &cobra.Command{
	Use:   "get <index>",
	Short: "Get task details by index",
	Long: `Get full details for a task by its index.

Requires --project-id flag.

Requires user authentication via 'andamio user login'.`,
	Args: cobra.ExactArgs(1),
	RunE: runTaskGet,
}

var projectTaskUpdateCmd = &cobra.Command{
	Use:   "update <index>",
	Short: "Update a task by index",
	Long: `Update a task's fields by its index.

Requires --project-id flag. Only specified flags are updated.

Requires user authentication via 'andamio user login'.`,
	Args: cobra.ExactArgs(1),
	RunE: runTaskUpdate,
}

var projectTaskDeleteCmd = &cobra.Command{
	Use:   "delete <index>",
	Short: "Delete a draft task by index",
	Long: `Delete a draft task by its index.

Requires --project-id flag.

Requires user authentication via 'andamio user login'.`,
	Args: cobra.ExactArgs(1),
	RunE: runTaskDelete,
}

var projectTaskComputeHashCmd = &cobra.Command{
	Use:   "compute-hash",
	Short: "Compute task hash from task data fields",
	Long: `Compute the Blake2b-256 hash of task data, matching the on-chain Plutus validator.

This is the same hash used for task verification on-chain. Use it to
pre-compute the task_hash before minting a task.

Provide task data either as individual flags or via --file pointing to a
task markdown file with frontmatter (same format as 'project task export').

No authentication required — this is a purely local computation.

Examples:
  andamio project task compute-hash --content "Build API" --lovelace 5000000 --expiration 2026-12-31
  andamio project task compute-hash --content "Earn XP" --lovelace 5000000 --expiration 2026-12-31 --token "policyid...,XP,50"
  andamio project task compute-hash --file tasks/my-project/001-build-api.md --output json`,
	Args: cobra.NoArgs,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Override parent's jwtAuthPreRunE — this command is purely local,
		// no auth needed. Only run the root output format setup.
		return rootCmd.PersistentPreRunE(cmd, args)
	},
	RunE: runTaskComputeHash,
}

var projectTaskVerifyHashCmd = &cobra.Command{
	Use:   "verify-hash <project-id>",
	Short: "Verify task hashes match computed hashes",
	Long: `Compute task hashes locally and compare against API-returned hashes.

Fetches all tasks for a project, computes the Blake2b-256 hash from
the task data (content, expiration, lovelace, assets), and reports
any mismatches. Useful for diagnosing hash issues with on-chain tasks.

Requires an API key or user authentication.

Examples:
  andamio project task verify-hash <project-id>
  andamio project task verify-hash <project-id> --output json`,
	Args: cobra.ExactArgs(1),
	RunE: runTaskVerifyHash,
}

func init() {
	projectCmd.AddCommand(projectTaskCmd)
	projectTaskCmd.AddCommand(projectTaskListCmd)
	projectTaskCmd.AddCommand(projectTaskCreateCmd)
	projectTaskCmd.AddCommand(projectTaskGetCmd)
	projectTaskCmd.AddCommand(projectTaskUpdateCmd)
	projectTaskCmd.AddCommand(projectTaskDeleteCmd)
	projectTaskCmd.AddCommand(projectTaskVerifyHashCmd)
	projectTaskCmd.AddCommand(projectTaskComputeHashCmd)

	// Compute-hash flags
	projectTaskComputeHashCmd.Flags().String("content", "", "Task content text (max 140 chars)")
	projectTaskComputeHashCmd.Flags().String("lovelace", "", "Lovelace reward amount, e.g. 5000000")
	projectTaskComputeHashCmd.Flags().String("expiration", "", "Expiration time in ISO 8601 format, e.g. 2026-12-31")
	projectTaskComputeHashCmd.Flags().StringArray("token", nil, `Native asset token (repeatable, format: "policy_id,asset_name,quantity")`)
	projectTaskComputeHashCmd.Flags().String("file", "", "Path to task markdown file with frontmatter")

	// Create flags
	projectTaskCreateCmd.Flags().String("title", "", "Task title (required)")
	projectTaskCreateCmd.MarkFlagRequired("title")
	projectTaskCreateCmd.Flags().String("lovelace", "", "Lovelace reward amount, e.g. 5000000 for 5 ADA (required)")
	projectTaskCreateCmd.MarkFlagRequired("lovelace")
	projectTaskCreateCmd.Flags().String("expiration", "", "Expiration time in ISO 8601 format, e.g. 2026-04-01T00:00:00Z (required)")
	projectTaskCreateCmd.MarkFlagRequired("expiration")
	projectTaskCreateCmd.Flags().String("content", "", "Plain text task description")
	projectTaskCreateCmd.Flags().String("content-file", "", "Markdown file for rich task content (converted to Tiptap JSON)")
	projectTaskCreateCmd.Flags().String("github-issue", "", "GitHub issue reference, e.g. org/repo#123 (prepended to title as [org/repo#123])")
	projectTaskCreateCmd.Flags().StringArray("token", nil, `Native asset token (repeatable, format: "policy_id,asset_name,quantity"). asset_name is auto-hex-encoded if not already hex`)

	// Get flags
	projectTaskGetCmd.Flags().String("project-id", "", "Project ID (required)")
	projectTaskGetCmd.MarkFlagRequired("project-id")

	// Update flags
	projectTaskUpdateCmd.Flags().String("project-id", "", "Project ID (required)")
	projectTaskUpdateCmd.MarkFlagRequired("project-id")
	projectTaskUpdateCmd.Flags().String("title", "", "New task title")
	projectTaskUpdateCmd.Flags().String("lovelace", "", "New lovelace reward amount")
	projectTaskUpdateCmd.Flags().String("expiration", "", "New expiration time (ISO 8601)")
	projectTaskUpdateCmd.Flags().String("content", "", "New plain text description")
	projectTaskUpdateCmd.Flags().String("content-file", "", "Markdown file for rich task content (converted to Tiptap JSON)")
	projectTaskUpdateCmd.Flags().StringArray("token", nil, `Native asset token (repeatable, format: "policy_id,asset_name,quantity"). asset_name is auto-hex-encoded if not already hex`)

	// Delete flags
	projectTaskDeleteCmd.Flags().String("project-id", "", "Project ID (required)")
	projectTaskDeleteCmd.MarkFlagRequired("project-id")
}

// managerProject holds the fields we need from the manager projects list response
type managerProject struct {
	ProjectID          string
	ContributorStateID string
	Title              string
}

// fetchManagerProjects calls POST /v2/project/manager/projects/list and returns parsed projects
func fetchManagerProjects(ctx context.Context, c *client.Client) ([]managerProject, error) {
	var resp map[string]interface{}
	if err := c.Post(ctx, "/api/v2/project/manager/projects/list", nil, &resp); err != nil {
		return nil, fmt.Errorf("failed to list manager projects: %w", err)
	}

	data, ok := resp["data"].([]interface{})
	if !ok || len(data) == 0 {
		return nil, nil
	}

	projects := make([]managerProject, 0, len(data))
	for _, item := range data {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		p := managerProject{}
		p.ProjectID, _ = m["project_id"].(string)
		p.ContributorStateID, _ = m["contributor_state_id"].(string)

		if content, ok := m["content"].(map[string]interface{}); ok {
			p.Title, _ = content["title"].(string)
		}

		if p.ProjectID != "" {
			projects = append(projects, p)
		}
	}
	return projects, nil
}

// resolveProject validates the project-id arg against the manager's project list
// and returns the matching project along with the full list (for policy ID lookups).
func resolveProject(ctx context.Context, c *client.Client, args []string) (*managerProject, []managerProject, error) {
	projects, err := fetchManagerProjects(ctx, c)
	if err != nil {
		return nil, nil, err
	}
	if len(projects) == 0 {
		return nil, nil, fmt.Errorf("no managed projects found")
	}

	for i := range projects {
		if projects[i].ProjectID == args[0] {
			return &projects[i], projects, nil
		}
	}
	return nil, nil, fmt.Errorf("project %s not found in your managed projects\n\nList your projects with:\n  andamio project list --output json", args[0])
}

// findProjectPolicyID looks up the contributor_state_id from a pre-fetched projects list
func findProjectPolicyID(projects []managerProject, projectID string) (string, error) {
	for _, p := range projects {
		if p.ProjectID == projectID {
			if p.ContributorStateID == "" {
				return "", fmt.Errorf("project %s has no contributor_state_id (may not be on-chain yet)", projectID)
			}
			return p.ContributorStateID, nil
		}
	}
	return "", fmt.Errorf("project %s not found in your managed projects", projectID)
}

// fetchTasks calls POST /v2/project/manager/tasks/list and returns the raw response
func fetchTasks(ctx context.Context, c *client.Client, projectID string) (map[string]interface{}, error) {
	body := map[string]string{"project_id": projectID}
	var resp map[string]interface{}
	if err := c.Post(ctx, "/api/v2/project/manager/tasks/list", body, &resp); err != nil {
		return nil, fmt.Errorf("failed to list tasks: %w", err)
	}
	return resp, nil
}

// extractTaskList extracts the data array from a tasks list response
func extractTaskList(resp map[string]interface{}) []map[string]interface{} {
	data, ok := resp["data"].([]interface{})
	if !ok {
		return nil
	}
	items := make([]map[string]interface{}, 0, len(data))
	for _, item := range data {
		if m, ok := item.(map[string]interface{}); ok {
			items = append(items, m)
		}
	}
	return items
}

func runTasksList(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	c := client.New(cfg)

	proj, _, err := resolveProject(ctx, c, args)
	if err != nil {
		return err
	}

	resp, err := fetchTasks(ctx, c, proj.ProjectID)
	if err != nil {
		return err
	}

	if output.GetFormat() == output.FormatJSON {
		return output.PrintJSON(resp)
	}

	items := extractTaskList(resp)
	if len(items) == 0 {
		fmt.Fprintln(os.Stderr, "No tasks found.")
		return nil
	}

	// Custom table output with task-relevant columns
	fmt.Printf("%-6s %-40s %-12s %12s %s\n", "Index", "Title", "Status", "Lovelace", "Expiration")
	fmt.Printf("%-6s %-40s %-12s %12s %s\n", "-----", "-----", "------", "--------", "----------")

	for _, item := range items {
		index := 0
		if v, ok := item["task_index"].(float64); ok {
			index = int(v)
		}
		status, _ := item["task_status"].(string)
		if status == "" {
			status, _ = item["source"].(string)
		}

		title := ""
		if content, ok := item["content"].(map[string]interface{}); ok {
			title, _ = content["title"].(string)
		}

		lovelace := int64(0)
		if v, ok := item["lovelace_amount"].(float64); ok {
			lovelace = int64(v)
		}

		expiration, _ := item["expiration"].(string)

		// Truncate long titles
		if len(title) > 38 {
			title = title[:35] + "..."
		}

		fmt.Printf("%-6d %-40s %-12s %12d %s\n", index, title, status, lovelace, expiration)
	}

	return nil
}

// parseExpiration converts an ISO 8601 date string to Unix milliseconds string for the API
func parseExpiration(exp string) (string, error) {
	// Try RFC3339 first (2026-04-01T00:00:00Z)
	t, err := time.Parse(time.RFC3339, exp)
	if err != nil {
		// Try date-only format (2026-04-01)
		t, err = time.Parse("2006-01-02", exp)
		if err != nil {
			return "", fmt.Errorf("invalid expiration format %q: use ISO 8601, e.g. 2026-04-01T00:00:00Z or 2026-04-01", exp)
		}
	}
	return strconv.FormatInt(t.UnixMilli(), 10), nil
}

// validateLovelace checks that a lovelace string is a valid non-negative integer
func validateLovelace(lovelace string) error {
	val, err := strconv.ParseInt(lovelace, 10, 64)
	if err != nil {
		return fmt.Errorf("--lovelace must be a non-negative integer, got %q", lovelace)
	}
	if val < 0 {
		return fmt.Errorf("--lovelace must be non-negative, got %d", val)
	}
	return nil
}

// parseTokenFlags parses --token flag values into TaskToken structs.
// Each value must be "policy_id,asset_name,quantity". Empty asset_name is allowed.
func parseTokenFlags(values []string) ([]TaskToken, error) {
	tokens := make([]TaskToken, 0, len(values))
	seen := make(map[string]bool)

	for _, v := range values {
		parts := strings.SplitN(v, ",", 3)
		if len(parts) != 3 {
			return nil, fmt.Errorf("invalid --token format %q: expected \"policy_id,asset_name,quantity\"\n  Example: --token \"722c475bebb10...,XP,50\"", v)
		}

		policyID := strings.TrimSpace(parts[0])
		assetName := hexEncodeAssetName(strings.TrimSpace(parts[1]))
		quantity := strings.TrimSpace(parts[2])

		if policyID == "" {
			return nil, fmt.Errorf("invalid --token format %q: policy_id cannot be empty", v)
		}
		if len(policyID) != 56 {
			return nil, fmt.Errorf("invalid --token policy_id %q: must be 56 hex characters", policyID)
		}
		if _, err := hex.DecodeString(policyID); err != nil {
			return nil, fmt.Errorf("invalid --token policy_id %q: must be hexadecimal", policyID)
		}
		if quantity == "" {
			return nil, fmt.Errorf("invalid --token format %q: quantity cannot be empty", v)
		}

		// Validate quantity is a non-negative integer
		val, err := strconv.ParseInt(quantity, 10, 64)
		if err != nil || val < 0 {
			return nil, fmt.Errorf("invalid --token quantity %q: must be a non-negative integer", quantity)
		}

		// Check for duplicates
		key := policyID + ":" + assetName
		if seen[key] {
			return nil, fmt.Errorf("duplicate token: policy_id %q + asset_name %q specified twice", policyID, assetName)
		}
		seen[key] = true

		tokens = append(tokens, TaskToken{
			PolicyID:  policyID,
			AssetName: assetName,
			Quantity:  quantity,
		})
	}

	return tokens, nil
}

func runTaskCreate(cmd *cobra.Command, args []string) error {
	title, _ := cmd.Flags().GetString("title")
	lovelace, _ := cmd.Flags().GetString("lovelace")
	expiration, _ := cmd.Flags().GetString("expiration")
	content, _ := cmd.Flags().GetString("content")
	contentFile, _ := cmd.Flags().GetString("content-file")
	githubIssue, _ := cmd.Flags().GetString("github-issue")
	isJSON := output.GetFormat() == output.FormatJSON

	if err := validateLovelace(lovelace); err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	c := client.New(cfg)

	ctx := cmd.Context()
	proj, projects, err := resolveProject(ctx, c, args)
	if err != nil {
		return err
	}

	policyID, err := findProjectPolicyID(projects, proj.ProjectID)
	if err != nil {
		return err
	}

	// Convert expiration to Unix ms
	expirationMs, err := parseExpiration(expiration)
	if err != nil {
		return err
	}

	// Prepend GitHub issue reference to title
	if githubIssue != "" {
		title = fmt.Sprintf("[%s] %s", githubIssue, title)
	}

	if !isJSON {
		fmt.Fprintf(os.Stderr, "Creating task: %s\n", title)
	}

	payload := map[string]interface{}{
		"contributor_state_id": policyID,
		"title":               title,
		"lovelace_amount":     lovelace,
		"expiration_time":     expirationMs,
	}
	if content != "" {
		payload["content"] = content
	}

	// Read Markdown file and convert to Tiptap JSON
	if contentFile != "" {
		contentJSON, err := readContentFile(contentFile)
		if err != nil {
			return err
		}
		payload["content_json"] = contentJSON
	}

	// Parse and add token rewards
	tokenStrs, _ := cmd.Flags().GetStringArray("token")
	if len(tokenStrs) > 0 {
		tokens, err := parseTokenFlags(tokenStrs)
		if err != nil {
			return err
		}
		payload["tokens"] = tokens
	}

	var resp map[string]interface{}
	if err := c.Post(ctx, "/api/v2/project/manager/task/create", payload, &resp); err != nil {
		return fmt.Errorf("failed to create task: %w", err)
	}

	if isJSON {
		return output.PrintJSON(resp)
	}

	fmt.Fprintf(os.Stderr, "Task created successfully.\n")
	return nil
}

func runTaskGet(cmd *cobra.Command, args []string) error {
	indexStr := args[0]
	projectID, _ := cmd.Flags().GetString("project-id")

	index, err := strconv.Atoi(indexStr)
	if err != nil {
		return fmt.Errorf("invalid index: %s", indexStr)
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	c := client.New(cfg)

	resp, err := fetchTasks(cmd.Context(), c, projectID)
	if err != nil {
		return err
	}

	items := extractTaskList(resp)
	for _, item := range items {
		taskIndex := 0
		if v, ok := item["task_index"].(float64); ok {
			taskIndex = int(v)
		}
		if taskIndex == index {
			return output.PrintJSON(item)
		}
	}

	return fmt.Errorf("task with index %d not found", index)
}

func runTaskUpdate(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	indexStr := args[0]
	projectID, _ := cmd.Flags().GetString("project-id")
	isJSON := output.GetFormat() == output.FormatJSON

	index, err := strconv.Atoi(indexStr)
	if err != nil {
		return fmt.Errorf("invalid index: %s", indexStr)
	}

	// Validate lovelace if provided
	if cmd.Flags().Changed("lovelace") {
		lovelace, _ := cmd.Flags().GetString("lovelace")
		if err := validateLovelace(lovelace); err != nil {
			return err
		}
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	c := client.New(cfg)

	projects, err := fetchManagerProjects(ctx, c)
	if err != nil {
		return err
	}
	policyID, err := findProjectPolicyID(projects, projectID)
	if err != nil {
		return err
	}

	payload := map[string]interface{}{
		"contributor_state_id": policyID,
		"index":               index,
	}

	// Only include flags that were explicitly set
	if cmd.Flags().Changed("title") {
		title, _ := cmd.Flags().GetString("title")
		payload["title"] = title
	}
	if cmd.Flags().Changed("lovelace") {
		lovelace, _ := cmd.Flags().GetString("lovelace")
		payload["lovelace_amount"] = lovelace
	}
	if cmd.Flags().Changed("expiration") {
		exp, _ := cmd.Flags().GetString("expiration")
		expirationMs, err := parseExpiration(exp)
		if err != nil {
			return err
		}
		payload["expiration_time"] = expirationMs
	}
	if cmd.Flags().Changed("content") {
		content, _ := cmd.Flags().GetString("content")
		payload["content"] = content
	}
	if cmd.Flags().Changed("content-file") {
		contentFile, _ := cmd.Flags().GetString("content-file")
		contentJSON, err := readContentFile(contentFile)
		if err != nil {
			return err
		}
		payload["content_json"] = contentJSON
	}
	if cmd.Flags().Changed("token") {
		tokenStrs, _ := cmd.Flags().GetStringArray("token")
		tokens, err := parseTokenFlags(tokenStrs)
		if err != nil {
			return err
		}
		payload["tokens"] = tokens
	}

	if !isJSON {
		fmt.Fprintf(os.Stderr, "Updating task %d...\n", index)
	}

	var resp map[string]interface{}
	if err := c.Post(ctx, "/api/v2/project/manager/task/update", payload, &resp); err != nil {
		return fmt.Errorf("failed to update task: %w", err)
	}

	if isJSON {
		return output.PrintJSON(resp)
	}

	fmt.Fprintf(os.Stderr, "Task %d updated successfully.\n", index)
	return nil
}

func runTaskDelete(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	indexStr := args[0]
	projectID, _ := cmd.Flags().GetString("project-id")
	isJSON := output.GetFormat() == output.FormatJSON

	index, err := strconv.Atoi(indexStr)
	if err != nil {
		return fmt.Errorf("invalid index: %s", indexStr)
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	c := client.New(cfg)

	projects, err := fetchManagerProjects(ctx, c)
	if err != nil {
		return err
	}
	policyID, err := findProjectPolicyID(projects, projectID)
	if err != nil {
		return err
	}

	payload := map[string]interface{}{
		"contributor_state_id": policyID,
		"index":               index,
	}

	if !isJSON {
		fmt.Fprintf(os.Stderr, "Deleting task %d...\n", index)
	}

	var resp map[string]interface{}
	if err := c.Post(ctx, "/api/v2/project/manager/task/delete", payload, &resp); err != nil {
		return fmt.Errorf("failed to delete task: %w", err)
	}

	if isJSON {
		return output.PrintJSON(resp)
	}

	fmt.Fprintf(os.Stderr, "Task %d deleted.\n", index)
	return nil
}

func runTaskVerifyHash(cmd *cobra.Command, args []string) error {
	projectID := args[0]
	isJSON := output.GetFormat() == output.FormatJSON

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	c := client.New(cfg)

	body := map[string]string{"project_id": projectID}
	var resp map[string]interface{}
	if err := c.Post(cmd.Context(), "/api/v2/project/user/tasks/list", body, &resp); err != nil {
		return fmt.Errorf("failed to list tasks: %w", err)
	}

	data, ok := resp["data"].([]interface{})
	if !ok || len(data) == 0 {
		if isJSON {
			return output.PrintJSON(map[string]interface{}{"results": []interface{}{}, "mismatches": 0})
		}
		fmt.Fprintln(os.Stderr, "No tasks found.")
		return nil
	}

	type verifyResult struct {
		TaskIndex      int              `json:"task_index"`
		Content        string           `json:"content"`
		APIHash        string           `json:"api_hash"`
		ComputedHash   string           `json:"computed_hash"`
		Match          bool             `json:"match"`
		ExpirationTime uint64           `json:"expiration_time"`
		Lovelace       uint64           `json:"lovelace_amount"`
		AssetCount     int              `json:"asset_count"`
		Assets         []cardano.NativeAsset `json:"assets,omitempty"`
		Error          string           `json:"error,omitempty"`
	}

	var results []verifyResult
	mismatches := 0

	for _, item := range data {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		// Try top-level task_index (merged tasks), then content.task_index
		// (chain_only tasks), then fall back to 1-based position.
		idx, _ := m["task_index"].(float64)
		if idx == 0 {
			if content, ok := m["content"].(map[string]interface{}); ok {
				idx, _ = content["task_index"].(float64)
			}
		}
		apiHash, _ := m["task_hash"].(string)
		if apiHash == "" {
			continue // skip non-on-chain tasks
		}

		// Extract task data
		onChainContent, _ := m["on_chain_content"].(string)
		projectContent := ""
		if onChainContent != "" {
			decoded, err := hex.DecodeString(onChainContent)
			if err == nil {
				projectContent = string(decoded)
			}
		}
		if projectContent == "" {
			if content, ok := m["content"].(map[string]interface{}); ok {
				projectContent, _ = content["title"].(string)
			}
		}

		expirationPosix, _ := m["expiration_posix"].(float64)
		lovelaceAmount, _ := m["lovelace_amount"].(float64)

		var nativeAssets []cardano.NativeAsset
		if assets, ok := m["assets"].([]interface{}); ok {
			for _, a := range assets {
				am, ok := a.(map[string]interface{})
				if !ok {
					continue
				}
				policyID, _ := am["policy_id"].(string)
				name, _ := am["name"].(string)
				amountStr, _ := am["amount"].(string)
				var quantity uint64
				if amountStr != "" {
					fmt.Sscanf(amountStr, "%d", &quantity)
				}
				tokenNameHex := hexEncodeAssetName(name)
				nativeAssets = append(nativeAssets, cardano.NativeAsset{
					PolicyID:  policyID,
					TokenName: tokenNameHex,
					Quantity:  quantity,
				})
			}
		}

		taskData := cardano.TaskData{
			ProjectContent: projectContent,
			ExpirationTime: uint64(expirationPosix),
			LovelaceAmount: uint64(lovelaceAmount),
			NativeAssets:   nativeAssets,
		}

		r := verifyResult{
			TaskIndex:      int(idx),
			Content:        projectContent,
			APIHash:        apiHash,
			ExpirationTime: uint64(expirationPosix),
			Lovelace:       uint64(lovelaceAmount),
			AssetCount:     len(nativeAssets),
			Assets:         nativeAssets,
		}

		computedHash, err := cardano.ComputeTaskHash(taskData)
		if err != nil {
			r.Error = err.Error()
		} else {
			r.ComputedHash = computedHash
			r.Match = computedHash == apiHash
		}

		if !r.Match {
			mismatches++
		}

		results = append(results, r)
	}

	if isJSON {
		return output.PrintJSON(map[string]interface{}{
			"results":    results,
			"total":      len(results),
			"mismatches": mismatches,
		})
	}

	// Text output
	for _, r := range results {
		status := "\u2713"
		if !r.Match {
			status = "\u2717"
		}
		fmt.Printf("%s Task %d: %q\n", status, r.TaskIndex, r.Content)
		if r.Error != "" {
			fmt.Printf("    Error: %s\n", r.Error)
			continue
		}
		if !r.Match {
			fmt.Printf("    API hash:      %s\n", r.APIHash)
			fmt.Printf("    Computed hash:  %s\n", r.ComputedHash)
			fmt.Printf("    Expiration: %d, Lovelace: %d, Assets: %d\n",
				r.ExpirationTime, r.Lovelace, r.AssetCount)
			for _, a := range r.Assets {
				fmt.Printf("      - %s / %s / %d\n", a.PolicyID, a.TokenName, a.Quantity)
			}
		}
	}

	fmt.Fprintf(os.Stderr, "\n%d tasks checked, %d mismatches\n", len(results), mismatches)
	return nil
}

func runTaskComputeHash(cmd *cobra.Command, args []string) error {
	isJSON := output.GetFormat() == output.FormatJSON

	fileFlag, _ := cmd.Flags().GetString("file")
	contentFlag, _ := cmd.Flags().GetString("content")
	lovelaceFlag, _ := cmd.Flags().GetString("lovelace")
	expirationFlag, _ := cmd.Flags().GetString("expiration")
	tokenStrs, _ := cmd.Flags().GetStringArray("token")

	hasFieldFlags := contentFlag != "" || lovelaceFlag != "" || expirationFlag != "" || len(tokenStrs) > 0

	if fileFlag != "" && hasFieldFlags {
		return fmt.Errorf("cannot use --file with --content, --lovelace, --expiration, or --token; choose one input method")
	}

	var content, lovelace, expiration string
	var nativeAssets []cardano.NativeAsset

	if fileFlag != "" {
		data, err := os.ReadFile(fileFlag)
		if err != nil {
			return fmt.Errorf("failed to read file: %w", err)
		}

		var fm TaskFrontmatter
		_, err = frontmatter.Parse(bytes.NewReader(data), &fm)
		if err != nil {
			return fmt.Errorf("failed to parse frontmatter: %w", err)
		}

		if fm.Title == "" {
			return fmt.Errorf("file %s is missing required 'title' in frontmatter (used as content)", fileFlag)
		}
		content = fm.Title

		if fm.Lovelace == "" {
			return fmt.Errorf("file %s is missing required 'lovelace' in frontmatter", fileFlag)
		}
		lovelace = fm.Lovelace

		if fm.ExpirationTime == "" {
			return fmt.Errorf("file %s is missing required 'expiration_time' in frontmatter", fileFlag)
		}
		expiration = fm.ExpirationTime

		for _, t := range fm.Tokens {
			assetNameHex := hexEncodeAssetName(t.AssetName)
			qty, err := strconv.ParseUint(t.Quantity, 10, 64)
			if err != nil {
				return fmt.Errorf("invalid token quantity %q: %w", t.Quantity, err)
			}
			nativeAssets = append(nativeAssets, cardano.NativeAsset{
				PolicyID:  t.PolicyID,
				TokenName: assetNameHex,
				Quantity:  qty,
			})
		}
	} else {
		if contentFlag == "" || lovelaceFlag == "" || expirationFlag == "" {
			return fmt.Errorf("--content, --lovelace, and --expiration are all required (or use --file); see 'andamio project task compute-hash --help'")
		}
		content = contentFlag
		lovelace = lovelaceFlag
		expiration = expirationFlag

		if len(tokenStrs) > 0 {
			tokens, err := parseTokenFlags(tokenStrs)
			if err != nil {
				return err
			}
			for _, t := range tokens {
				qty, err := strconv.ParseUint(t.Quantity, 10, 64)
				if err != nil {
					return fmt.Errorf("invalid token quantity %q: %w", t.Quantity, err)
				}
				nativeAssets = append(nativeAssets, cardano.NativeAsset{
					PolicyID:  t.PolicyID,
					TokenName: t.AssetName,
					Quantity:  qty,
				})
			}
		}
	}

	if err := validateLovelace(lovelace); err != nil {
		return err
	}

	expirationMs, err := parseExpiration(expiration)
	if err != nil {
		return err
	}
	expMs, _ := strconv.ParseUint(expirationMs, 10, 64)
	lovelaceVal, _ := strconv.ParseUint(lovelace, 10, 64)

	taskData := cardano.TaskData{
		ProjectContent: content,
		ExpirationTime: expMs,
		LovelaceAmount: lovelaceVal,
		NativeAssets:   nativeAssets,
	}

	hash, err := cardano.ComputeTaskHash(taskData)
	if err != nil {
		return fmt.Errorf("hash computation failed: %w", err)
	}

	if isJSON {
		tokensOut := make([]map[string]interface{}, 0, len(nativeAssets))
		for _, a := range nativeAssets {
			tokensOut = append(tokensOut, map[string]interface{}{
				"policy_id":  a.PolicyID,
				"token_name": a.TokenName,
				"quantity":   a.Quantity,
			})
		}
		return output.PrintJSON(map[string]interface{}{
			"task_hash": hash,
			"fields": map[string]interface{}{
				"content":       content,
				"lovelace":      lovelaceVal,
				"expiration_ms": expMs,
				"tokens":        tokensOut,
			},
		})
	}

	fmt.Printf("%s\n", hash)
	fmt.Fprintf(os.Stderr, "Content: %s\n", content)
	fmt.Fprintf(os.Stderr, "Lovelace: %d, Expiration: %d ms, Tokens: %d\n", lovelaceVal, expMs, len(nativeAssets))
	return nil
}

// readContentFile reads a Markdown file and converts it to Tiptap JSON for content_json.
func readContentFile(path string) (interface{}, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read content file %s: %w", path, err)
	}
	body := strings.TrimSpace(string(data))
	if body == "" {
		return nil, fmt.Errorf("content file %s is empty", path)
	}
	contentJSON, err := markdownToTiptap(body, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to convert Markdown to Tiptap: %w", err)
	}
	return contentJSON, nil
}
