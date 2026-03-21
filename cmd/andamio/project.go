package main

import (
	"fmt"
	"net/url"
	"os"

	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/Andamio-Platform/andamio-cli/internal/config"
	"github.com/Andamio-Platform/andamio-cli/internal/output"
	"github.com/spf13/cobra"
)

var projectCmd = &cobra.Command{
	Use:   "project",
	Short: "Manage projects",
}

var projectListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available projects",
	RunE: func(cmd *cobra.Command, args []string) error {
		return printList("/api/v2/project/user/projects/list", "No projects found.", "content.title", "project_id", false)
	},
}

var projectGetCmd = &cobra.Command{
	Use:   "get <project-id>",
	Short: "Get project details",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return getJSON("/api/v2/project/user/project/" + url.PathEscape(args[0]))
	},
}

var projectTasksPublicCmd = &cobra.Command{
	Use:   "tasks <project-id>",
	Short: "List tasks for a project (public view)",
	Long: `List tasks for a project. Unlike 'project task list' (manager endpoint),
this uses the public user endpoint and does not require manager role.

Examples:
  andamio project tasks <project-id>
  andamio project tasks <project-id> --output json`,
	Args: cobra.ExactArgs(1),
	RunE: runProjectTasksPublic,
}

func init() {
	rootCmd.AddCommand(projectCmd)
	projectCmd.AddCommand(projectListCmd)
	projectCmd.AddCommand(projectGetCmd)
	projectCmd.AddCommand(projectTasksPublicCmd)
}

func runProjectTasksPublic(cmd *cobra.Command, args []string) error {
	projectID := args[0]
	isJSON := output.GetFormat() == output.FormatJSON

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	c := client.New(cfg)
	payload := map[string]string{"project_id": projectID}
	var resp map[string]interface{}
	if err := c.Post("/api/v2/project/user/tasks/list", payload, &resp); err != nil {
		return fmt.Errorf("failed to list tasks: %w", err)
	}

	if isJSON {
		return output.PrintJSON(resp)
	}

	data, ok := resp["data"].([]interface{})
	if !ok || len(data) == 0 {
		fmt.Fprintln(os.Stderr, "No tasks found.")
		return nil
	}

	items := make([]map[string]interface{}, 0, len(data))
	for _, item := range data {
		if m, ok := item.(map[string]interface{}); ok {
			items = append(items, m)
		}
	}

	return output.PrintList(items, "content.title", "task_index")
}
