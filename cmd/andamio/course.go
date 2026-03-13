package main

import (
	"encoding/json"
	"fmt"

	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/Andamio-Platform/andamio-cli/internal/config"
	"github.com/spf13/cobra"
)

var courseCmd = &cobra.Command{
	Use:   "course",
	Short: "Manage courses",
	Long:  `List, view, and export courses from the Andamio platform.`,
}

var courseListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available courses",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}

		c := client.New(cfg)
		var courses []map[string]interface{}
		if err := c.Get("/api/v2/courses", &courses); err != nil {
			return err
		}

		if len(courses) == 0 {
			fmt.Println("No courses found.")
			return nil
		}

		for _, course := range courses {
			fmt.Printf("- %s (%s)\n", course["title"], course["id"])
		}
		return nil
	},
}

var courseExportCmd = &cobra.Command{
	Use:   "export [course-id]",
	Short: "Export a course",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		courseID := args[0]

		cfg, err := config.Load()
		if err != nil {
			return err
		}

		c := client.New(cfg)
		var course map[string]interface{}
		if err := c.Get("/api/v2/courses/"+courseID+"/export", &course); err != nil {
			return err
		}

		output, err := json.MarshalIndent(course, "", "  ")
		if err != nil {
			return err
		}

		fmt.Println(string(output))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(courseCmd)
	courseCmd.AddCommand(courseListCmd)
	courseCmd.AddCommand(courseExportCmd)
}
