package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/Andamio-Platform/andamio-cli/internal/config"
	"github.com/Andamio-Platform/andamio-cli/internal/output"
	"github.com/adrg/frontmatter"
	"github.com/spf13/cobra"
)

var projectTaskImportCmd = &cobra.Command{
	Use:   "import <project-id>",
	Short: "Import tasks from local Markdown files",
	Long: `Import tasks from local Markdown files with YAML frontmatter.

Reads .md files from tasks/<project-slug>/ directory.
Files without an 'index' in frontmatter create new tasks.
Files with an 'index' update existing tasks.

Non-DRAFT tasks are skipped with a warning.

Find your project IDs with: andamio project list --output json

Requires user authentication via 'andamio user login'.`,
	Args: cobra.ExactArgs(1),
	RunE: runTaskImport,
}

func init() {
	projectTaskCmd.AddCommand(projectTaskImportCmd)
	projectTaskImportCmd.Flags().Bool("dry-run", false, "Preview API payloads without sending")
}

// TaskFrontmatter defines the YAML frontmatter structure for task files
type TaskFrontmatter struct {
	Title          string      `yaml:"title"`
	Lovelace       string      `yaml:"lovelace"`
	ExpirationTime string      `yaml:"expiration_time"`
	Tokens         []TaskToken `yaml:"tokens"`
	Index          *int        `yaml:"index"`
	Status         string      `yaml:"status"`
	Description    string      `yaml:"description"`
}

// TaskToken represents a token reward in frontmatter
type TaskToken struct {
	PolicyID  string `yaml:"policy_id" json:"policy_id"`
	AssetName string `yaml:"asset_name" json:"asset_name"`
	Quantity  string `yaml:"quantity" json:"quantity"`
}

// TaskImportResult holds the result for structured output
type TaskImportResult struct {
	ProjectID string `json:"project_id"`
	Directory string `json:"directory"`
	Created   int    `json:"tasks_created"`
	Updated   int    `json:"tasks_updated"`
	Skipped   int    `json:"tasks_skipped"`
	Errors    int    `json:"errors"`
	DryRun    bool   `json:"dry_run,omitempty"`
}

func runTaskImport(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	isJSON := output.GetFormat() == output.FormatJSON

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	c := client.New(cfg)

	// Single fetch: resolve project, policy ID, and slug from one API call
	proj, projects, err := resolveProject(ctx, c, args)
	if err != nil {
		return err
	}

	policyID, err := findProjectPolicyID(projects, proj.ProjectID)
	if err != nil {
		return err
	}

	projectSlug := projectSlugFromList(projects, proj.ProjectID)
	taskDir := filepath.Join("tasks", projectSlug)
	absDir, err := filepath.Abs(taskDir)
	if err != nil {
		return fmt.Errorf("invalid directory: %w", err)
	}

	// Read .md files
	entries, err := os.ReadDir(absDir)
	if err != nil {
		return fmt.Errorf("failed to read directory %s: %w", taskDir, err)
	}

	var mdFiles []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			mdFiles = append(mdFiles, e.Name())
		}
	}

	if len(mdFiles) == 0 {
		if isJSON {
			return output.PrintJSON(TaskImportResult{ProjectID: proj.ProjectID, Directory: taskDir, DryRun: dryRun})
		}
		fmt.Fprintf(os.Stderr, "No .md files found in %s/\n", taskDir)
		return nil
	}

	// Fetch existing tasks to check status for updates
	var existingTasks []map[string]interface{}
	resp, err := fetchTasks(ctx, c, proj.ProjectID)
	if err == nil {
		existingTasks = extractTaskList(resp)
	}

	if !isJSON {
		action := "Importing"
		if dryRun {
			action = "Dry-run importing"
		}
		fmt.Fprintf(os.Stderr, "%s %d task files from %s/\n", action, len(mdFiles), taskDir)
	}

	result := TaskImportResult{
		ProjectID: proj.ProjectID,
		Directory: taskDir,
		DryRun:    dryRun,
	}

	for _, filename := range mdFiles {
		filePath := filepath.Join(absDir, filename)
		action, err := importTaskFile(ctx, c, filePath, filename, policyID, existingTasks, dryRun, isJSON)
		if err != nil {
			result.Errors++
			if !isJSON {
				fmt.Fprintf(os.Stderr, "  %s: ERROR: %v\n", filename, err)
			}
			continue
		}
		switch action {
		case "created":
			result.Created++
		case "updated":
			result.Updated++
		case "skipped":
			result.Skipped++
		}
	}

	if isJSON {
		return output.PrintJSON(result)
	}

	fmt.Fprintf(os.Stderr, "\nImport complete: %d created, %d updated, %d skipped, %d errors\n",
		result.Created, result.Updated, result.Skipped, result.Errors)
	return nil
}

// importTaskFile processes a single task file and returns the action taken
func importTaskFile(ctx context.Context, c *client.Client, filePath, filename, policyID string, existingTasks []map[string]interface{}, dryRun, isJSON bool) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	var fm TaskFrontmatter
	rest, err := frontmatter.Parse(bytes.NewReader(data), &fm)
	if err != nil {
		return "", fmt.Errorf("failed to parse frontmatter: %w", err)
	}

	if fm.Title == "" {
		return "", fmt.Errorf("missing required field: title")
	}
	if fm.Lovelace == "" {
		return "", fmt.Errorf("missing required field: lovelace")
	}
	if fm.ExpirationTime == "" {
		return "", fmt.Errorf("missing required field: expiration_time")
	}

	// Validate lovelace
	if err := validateLovelace(fm.Lovelace); err != nil {
		return "", err
	}

	// Convert expiration to Unix ms
	expirationMs, err := parseExpiration(fm.ExpirationTime)
	if err != nil {
		return "", err
	}

	// Convert Markdown body to Tiptap JSON
	bodyStr := strings.TrimSpace(string(rest))
	var contentJSON interface{}
	if bodyStr != "" {
		contentJSON, err = markdownToTiptap(bodyStr, nil)
		if err != nil {
			return "", fmt.Errorf("failed to convert Markdown to Tiptap: %w", err)
		}
	}

	if fm.Index == nil {
		// New task — create
		return createTaskFromFile(ctx, c, fm, policyID, expirationMs, contentJSON, filename, dryRun, isJSON)
	}

	// Existing task — check status before updating
	index := *fm.Index
	for _, existing := range existingTasks {
		if v, ok := existing["task_index"].(float64); ok && int(v) == index {
			status, _ := existing["task_status"].(string)
			if status == "" {
				status, _ = existing["source"].(string)
			}
			if status != "" && status != "DRAFT" {
				if !isJSON {
					reason := "only DRAFT tasks can be updated"
					switch status {
					case "ON_CHAIN":
						reason = "task is on-chain and immutable"
					case "PENDING_TX":
						reason = "task has a pending transaction"
					case "CANCELLED":
						reason = "task was removed on-chain"
					}
					fmt.Fprintf(os.Stderr, "  %s: SKIPPED (task %d is %s — %s)\n", filename, index, status, reason)
				}
				return "skipped", nil
			}
		}
	}

	return updateTaskFromFile(ctx, c, fm, policyID, expirationMs, contentJSON, index, filename, dryRun, isJSON)
}

func createTaskFromFile(ctx context.Context, c *client.Client, fm TaskFrontmatter, policyID, expirationMs string, contentJSON interface{}, filename string, dryRun, isJSON bool) (string, error) {
	payload := map[string]interface{}{
		"contributor_state_id": policyID,
		"title":                   fm.Title,
		"lovelace_amount":         fm.Lovelace,
		"expiration_time":         expirationMs,
	}

	if fm.Description != "" {
		payload["content"] = fm.Description
	}
	if contentJSON != nil {
		payload["content_json"] = contentJSON
	}
	if len(fm.Tokens) > 0 {
		tokens := make([]TaskToken, len(fm.Tokens))
		for i, t := range fm.Tokens {
			tokens[i] = TaskToken{PolicyID: t.PolicyID, AssetName: hexEncodeAssetName(t.AssetName), Quantity: t.Quantity}
		}
		payload["tokens"] = tokens
	}

	if dryRun {
		data, _ := json.MarshalIndent(payload, "", "  ")
		if !isJSON {
			fmt.Fprintf(os.Stderr,"  %s: CREATE (dry-run)\n%s\n", filename, string(data))
		}
		return "created", nil
	}

	var resp map[string]interface{}
	if err := c.Post(ctx, "/api/v2/project/manager/task/create", payload, &resp); err != nil {
		return "", fmt.Errorf("failed to create: %w", err)
	}

	if !isJSON {
		fmt.Fprintf(os.Stderr,"  %s: CREATED %q\n", filename, fm.Title)
	}
	return "created", nil
}

func updateTaskFromFile(ctx context.Context, c *client.Client, fm TaskFrontmatter, policyID, expirationMs string, contentJSON interface{}, index int, filename string, dryRun, isJSON bool) (string, error) {
	payload := map[string]interface{}{
		"contributor_state_id": policyID,
		"index":                   index,
		"title":                   fm.Title,
		"lovelace_amount":         fm.Lovelace,
		"expiration_time":         expirationMs,
	}

	if fm.Description != "" {
		payload["content"] = fm.Description
	}
	if contentJSON != nil {
		payload["content_json"] = contentJSON
	}
	if len(fm.Tokens) > 0 {
		tokens := make([]TaskToken, len(fm.Tokens))
		for i, t := range fm.Tokens {
			tokens[i] = TaskToken{PolicyID: t.PolicyID, AssetName: hexEncodeAssetName(t.AssetName), Quantity: t.Quantity}
		}
		payload["tokens"] = tokens
	}

	if !isJSON && !dryRun {
		fmt.Fprintf(os.Stderr,"  %s: WARNING: updating existing task %d %q\n", filename, index, fm.Title)
	}

	if dryRun {
		data, _ := json.MarshalIndent(payload, "", "  ")
		if !isJSON {
			fmt.Fprintf(os.Stderr,"  %s: UPDATE task %d (dry-run)\n%s\n", filename, index, string(data))
		}
		return "updated", nil
	}

	var resp map[string]interface{}
	if err := c.Post(ctx, "/api/v2/project/manager/task/update", payload, &resp); err != nil {
		return "", fmt.Errorf("failed to update: %w", err)
	}

	if !isJSON {
		fmt.Fprintf(os.Stderr,"  %s: UPDATED task %d %q\n", filename, index, fm.Title)
	}
	return "updated", nil
}
