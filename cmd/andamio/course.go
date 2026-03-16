package main

import (
	"fmt"
	"net/url"

	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/Andamio-Platform/andamio-cli/internal/config"
	"github.com/Andamio-Platform/andamio-cli/internal/output"
	"github.com/spf13/cobra"
)

var courseCmd = &cobra.Command{
	Use:   "course",
	Short: "Manage courses",
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
		var response map[string]interface{}
		if err := c.Get("/api/v2/course/user/courses/list", &response); err != nil {
			return err
		}

		data, ok := response["data"].([]interface{})
		if !ok || len(data) == 0 {
			fmt.Println("No courses found.")
			return nil
		}

		items := make([]map[string]interface{}, 0, len(data))
		for _, item := range data {
			if course, ok := item.(map[string]interface{}); ok {
				items = append(items, course)
			}
		}

		return output.PrintList(items, "content.title", "course_id")
	},
}

var courseGetCmd = &cobra.Command{
	Use:   "get <course-id>",
	Short: "Get course details",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return getJSON("/api/v2/course/user/course/get/" + url.PathEscape(args[0]))
	},
}

var courseModulesCmd = &cobra.Command{
	Use:   "modules <course-id>",
	Short: "List modules for a course",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return getJSON("/api/v2/course/user/modules/" + url.PathEscape(args[0]))
	},
}

var courseSltsCmd = &cobra.Command{
	Use:   "slts <course-id> <module-code>",
	Short: "List SLTs for a course module",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return getJSON("/api/v2/course/user/slts/" + url.PathEscape(args[0]) + "/" + url.PathEscape(args[1]))
	},
}

var courseLessonCmd = &cobra.Command{
	Use:   "lesson <course-id> <module-code> <slt-index>",
	Short: "Get lesson content",
	Args:  cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		return getJSON("/api/v2/course/user/lesson/" + url.PathEscape(args[0]) + "/" + url.PathEscape(args[1]) + "/" + url.PathEscape(args[2]))
	},
}

var courseAssignmentCmd = &cobra.Command{
	Use:   "assignment <course-id> <module-code>",
	Short: "Get assignment for a course module",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return getJSON("/api/v2/course/user/assignment/" + url.PathEscape(args[0]) + "/" + url.PathEscape(args[1]))
	},
}

var courseIntroCmd = &cobra.Command{
	Use:   "intro <course-id> <module-code>",
	Short: "Get introduction for a course module",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return getJSON("/api/v2/course/user/introduction/" + url.PathEscape(args[0]) + "/" + url.PathEscape(args[1]))
	},
}

func init() {
	rootCmd.AddCommand(courseCmd)
	courseCmd.AddCommand(courseListCmd)
	courseCmd.AddCommand(courseGetCmd)
	courseCmd.AddCommand(courseModulesCmd)
	courseCmd.AddCommand(courseSltsCmd)
	courseCmd.AddCommand(courseLessonCmd)
	courseCmd.AddCommand(courseAssignmentCmd)
	courseCmd.AddCommand(courseIntroCmd)
}

// getJSON is a helper for simple GET endpoints that return JSON
func getJSON(path string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	c := client.New(cfg)
	var result map[string]interface{}
	if err := c.Get(path, &result); err != nil {
		return err
	}

	return output.PrintJSON(result)
}

// postJSON is a helper for simple POST endpoints that return JSON (no body)
func postJSON(path string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	c := client.New(cfg)
	var result map[string]interface{}
	if err := c.Post(path, nil, &result); err != nil {
		return err
	}

	return output.PrintJSON(result)
}
