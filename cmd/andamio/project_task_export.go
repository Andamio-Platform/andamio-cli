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
	RunE: runTaskExport,
}

func init() {
	projectTaskCmd.AddCommand(projectTaskExportCmd)
}

// TaskExportResult holds the result for structured output
type TaskExportResult struct {
	ProjectID    string   `json:"project_id"`
	Directory    string   `json:"directory"`
	TasksExported int     `json:"tasks_exported"`
	Files        []string `json:"files"`
}

// formatExpirationISO converts Unix ms (float64 from JSON) to ISO 8601 string
func formatExpirationISO(posix float64) string {
	ms := int64(posix)
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

	proj, _, err := resolveProject(c, args)
	if err != nil {
		return err
	}

	projectSlug := slugify(proj.Title)
	if proj.Title == "" {
		projectSlug = slugify(proj.ProjectID)
	}
	policyID := proj.ContributorStateID

	resp, err := fetchTasks(c, proj.ProjectID)
	if err != nil {
		return err
	}

	items := extractTaskList(resp)
	if len(items) == 0 {
		if isJSON {
			return output.PrintJSON(TaskExportResult{
				ProjectID: proj.ProjectID,
				Directory: fmt.Sprintf("tasks/%s", projectSlug),
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
		ProjectID: proj.ProjectID,
		Directory: outDir,
	}

	for _, item := range items {
		filename, err := exportTask(absDir, item, proj.ProjectID, policyID)
		if err != nil {
			if !isJSON {
				fmt.Printf("  Warning: failed to export task: %v\n", err)
			}
			continue
		}
		result.Files = append(result.Files, filename)
		result.TasksExported++
		if !isJSON {
			fmt.Printf("  %s\n", filename)
		}
	}

	if isJSON {
		return output.PrintJSON(result)
	}

	fmt.Printf("\nExported %d tasks to %s/\n", result.TasksExported, outDir)
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

	// Derive filename: prepend index to prevent collisions from duplicate slugified titles
	slug := slugify(title)
	var filename string
	if index >= 0 {
		filename = fmt.Sprintf("%03d-%s.md", index, slug)
	} else {
		filename = slug + ".md"
	}
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

// projectSlugFromList finds a project's slug from a pre-fetched projects list
func projectSlugFromList(projects []managerProject, projectID string) string {
	for _, p := range projects {
		if p.ProjectID == projectID && p.Title != "" {
			return slugify(p.Title)
		}
	}
	return slugify(projectID)
}
