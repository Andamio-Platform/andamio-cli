package main

import (
	"fmt"
	"net/url"

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
		cfg, err := config.Load()
		if err != nil {
			return err
		}

		c := client.New(cfg)
		var response map[string]interface{}
		if err := c.Get("/api/v2/project/user/projects/list", &response); err != nil {
			return err
		}

		data, ok := response["data"].([]interface{})
		if !ok || len(data) == 0 {
			fmt.Println("No projects found.")
			return nil
		}

		items := make([]map[string]interface{}, 0, len(data))
		for _, item := range data {
			if project, ok := item.(map[string]interface{}); ok {
				items = append(items, project)
			}
		}

		return output.PrintList(items, "content.title", "project_id")
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
