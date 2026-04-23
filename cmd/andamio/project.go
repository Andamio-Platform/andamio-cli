package main

import (
	"net/url"

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
		return printList(cmd.Context(), "/api/v2/project/user/projects/list", "No projects found.", "content.title", "project_id", false)
	},
}

var projectGetCmd = &cobra.Command{
	Use:   "get <project-id>",
	Short: "Get project details",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return getJSON(cmd.Context(), "/api/v2/project/user/project/"+url.PathEscape(args[0]))
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
	return printListPost(
		cmd.Context(),
		"/api/v2/project/user/tasks/list",
		map[string]string{"project_id": args[0]},
		"No tasks found.",
		"content.title", "task_index",
	)
}
