package main

import (
	"github.com/Andamio-Platform/andamio-cli/internal/apierr"
	"github.com/Andamio-Platform/andamio-cli/internal/config"
	"github.com/spf13/cobra"
)

var managerCmd = &cobra.Command{
	Use:   "manager",
	Short: "Project manager operations (requires user login)",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
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
	},
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
