package main

import "github.com/spf13/cobra"

var managerCmd = &cobra.Command{
	Use:               "manager",
	Short:             "Project manager operations (requires user login)",
	PersistentPreRunE: jwtAuthPreRunE,
}

var managerProjectsCmd = &cobra.Command{
	Use:   "projects",
	Short: "List projects where you are a manager",
	RunE: func(cmd *cobra.Command, args []string) error {
		return printList(
			"/api/v2/project/manager/projects/list",
			"No projects found where you are a manager.",
			"content.title", "project_id", true,
		)
	},
}

func init() {
	rootCmd.AddCommand(managerCmd)
	managerCmd.AddCommand(managerProjectsCmd)
}
