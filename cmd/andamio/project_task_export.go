package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/Andamio-Platform/andamio-cli/internal/config"
	"github.com/Andamio-Platform/andamio-cli/internal/output"
	"github.com/spf13/cobra"
)

var projectTaskExportCmd = &cobra.Command{
	Use:   "export [project-id]",
	Short: "Export tasks to local Markdown files",
	Long: `Export all tasks for a project to local Markdown files with YAML frontmatter.

Files are written to tasks/<project-slug>/ with one file per task.
Each file contains YAML frontmatter (title, lovelace, expiration, tokens, etc.)
and the task content as Markdown.

If project-id is omitted, lists your managed projects and prompts for selection.

Requires user authentication via 'andamio user login'.`,
	Args: cobra.MaximumNArgs(1),
	PreRunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		if !cfg.HasUserAuth() {
			return fmt.Errorf("not authenticated. Run 'andamio user login' first")
		}
		return nil
	},
	RunE: runTaskExport,
}

func init() {
	projectTaskCmd.AddCommand(projectTaskExportCmd)
}

// TaskExportResult holds the result for structured output
type TaskExportResult struct {
	ProjectID string `json:"project_id"`
	Directory string `json:"directory"`
	Tasks     int    `json:"tasks_exported"`
	Files     []string `json:"files"`
}

// formatExpirationISO converts Unix ms (int64 or float64) to ISO 8601 string
func formatExpirationISO(posix interface{}) string {
	var ms int64
	switch v := posix.(type) {
	case float64:
		ms = int64(v)
	case int64:
		ms = v
	default:
		return ""
	}
	if ms == 0 {
		return ""
	}
	t := time.UnixMilli(ms)
	return t.UTC().Format(time.RFC3339)
}

func runTaskExport(cmd *cobra.Command, args []string) error {
	isJSON := output.GetFormat() == output.FormatJSON

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	c := client.New(cfg)

	projectID, err := resolveProjectID(c, args)
	if err != nil {
		return err
	}

	// Get project title for directory name
	projectSlug := slugify(projectID) // fallback
	projects, err := fetchManagerProjects(c)
	if err == nil {
		for _, p := range projects {
			if p.ProjectID == projectID && p.Title != "" {
				projectSlug = slugify(p.Title)
				break
			}
		}
	}

	// Get contributor_state_id for metadata
	policyID := ""
	if projects != nil {
		for _, p := range projects {
			if p.ProjectID == projectID {
				policyID = p.ContributorStateID
				break
			}
		}
	}

	resp, err := fetchTasks(c, projectID)
	if err != nil {
		return err
	}

	items := extractTaskList(resp)
	if len(items) == 0 {
		if isJSON {
			return output.PrintJSON(TaskExportResult{
				ProjectID: projectID,
				Directory: fmt.Sprintf("tasks/%s", projectSlug),
				Tasks:     0,
			})
		}
		fmt.Println("No tasks to export.")
		return nil
	}

	// Create output directory
	outDir := filepath.Join("tasks", projectSlug)
	absDir, err := filepath.Abs(outDir)
	if err != nil {
		return fmt.Errorf("invalid output directory: %w", err)
	}
	if err := os.MkdirAll(absDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	if !isJSON {
		fmt.Printf("Exporting %d tasks to %s/\n", len(items), outDir)
	}

	result := TaskExportResult{
		ProjectID: projectID,
		Directory: outDir,
	}

	for _, item := range items {
		filename, err := exportTask(absDir, item, projectID, policyID)
		if err != nil {
			if !isJSON {
				fmt.Printf("  Warning: failed to export task: %v\n", err)
			}
			continue
		}
		result.Files = append(result.Files, filename)
		result.Tasks++
		if !isJSON {
			fmt.Printf("  %s\n", filename)
		}
	}

	if isJSON {
		return output.PrintJSON(result)
	}

	fmt.Printf("\nExported %d tasks to %s/\n", result.Tasks, outDir)
	return nil
}

// exportTask writes a single task to a Markdown file and returns the filename
func exportTask(dir string, item map[string]interface{}, projectID, policyID string) (string, error) {
	// Extract task fields
	title := ""
	description := ""
	var contentJSON interface{}
	if content, ok := item["content"].(map[string]interface{}); ok {
		title, _ = content["title"].(string)
		description, _ = content["description"].(string)
		contentJSON = content["content_json"]
	}

	index := -1
	if v, ok := item["task_index"].(float64); ok {
		index = int(v)
	}

	status, _ := item["task_status"].(string)
	if status == "" {
		status, _ = item["source"].(string)
	}

	lovelace := int64(0)
	if v, ok := item["lovelace_amount"].(float64); ok {
		lovelace = int64(v)
	}

	expirationISO := ""
	if v, ok := item["expiration"].(string); ok && v != "" {
		expirationISO = v
	} else if v, ok := item["expiration_posix"].(float64); ok {
		expirationISO = formatExpirationISO(v)
	}

	contributorStateID, _ := item["contributor_state_id"].(string)
	if contributorStateID == "" {
		contributorStateID = policyID
	}

	// Build frontmatter
	var fm strings.Builder
	fm.WriteString("---\n")
	fm.WriteString(fmt.Sprintf("title: %q\n", title))
	fm.WriteString(fmt.Sprintf("lovelace: %q\n", fmt.Sprintf("%d", lovelace)))
	if expirationISO != "" {
		fm.WriteString(fmt.Sprintf("expiration_time: %q\n", expirationISO))
	}

	// Write tokens if present
	if assets, ok := item["assets"].([]interface{}); ok && len(assets) > 0 {
		fm.WriteString("tokens:\n")
		for _, asset := range assets {
			a, ok := asset.(map[string]interface{})
			if !ok {
				continue
			}
			policyIDToken, _ := a["policy_id"].(string)
			assetName, _ := a["asset_name"].(string)
			quantity, _ := a["quantity"].(string)
			if quantity == "" {
				if v, ok := a["quantity"].(float64); ok {
					quantity = fmt.Sprintf("%.0f", v)
				}
			}
			fm.WriteString(fmt.Sprintf("  - policy_id: %q\n", policyIDToken))
			fm.WriteString(fmt.Sprintf("    asset_name: %q\n", assetName))
			fm.WriteString(fmt.Sprintf("    quantity: %q\n", quantity))
		}
	}

	if index >= 0 {
		fm.WriteString(fmt.Sprintf("index: %d\n", index))
	}
	if status != "" {
		fm.WriteString(fmt.Sprintf("status: %q\n", status))
	}
	fm.WriteString(fmt.Sprintf("project_id: %q\n", projectID))
	if contributorStateID != "" {
		fm.WriteString(fmt.Sprintf("project_state_policy_id: %q\n", contributorStateID))
	}
	if description != "" {
		fm.WriteString(fmt.Sprintf("description: %q\n", description))
	}
	fm.WriteString("---\n\n")

	// Convert content_json to Markdown if present
	body := ""
	if contentJSON != nil {
		if contentMap, ok := contentJSON.(map[string]interface{}); ok {
			md, _ := tiptapToMarkdown(contentMap)
			body = strings.TrimSpace(md)
		}
	}

	// Derive filename from title
	slug := slugify(title)
	filename := slug + ".md"
	filePath := filepath.Join(dir, filename)

	fileContent := fm.String()
	if body != "" {
		fileContent += body + "\n"
	}

	if err := writeFileAtomic(filePath, []byte(fileContent)); err != nil {
		return "", err
	}

	return filename, nil
}

// getProjectSlug returns a slug for a project by looking up its title
func getProjectSlug(c *client.Client, projectID string) string {
	projects, err := fetchManagerProjects(c)
	if err != nil {
		return slugify(projectID)
	}
	for _, p := range projects {
		if p.ProjectID == projectID && p.Title != "" {
			return slugify(p.Title)
		}
	}
	return slugify(projectID)
}
