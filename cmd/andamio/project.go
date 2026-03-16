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

func init() {
	rootCmd.AddCommand(projectCmd)
	projectCmd.AddCommand(projectListCmd)
	projectCmd.AddCommand(projectGetCmd)
}
