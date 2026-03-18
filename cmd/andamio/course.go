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

var courseCmd = &cobra.Command{
	Use:   "course",
	Short: "Manage courses",
}

var courseListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available courses",
	RunE: func(cmd *cobra.Command, args []string) error {
		return printList("/api/v2/course/user/courses/list", "No courses found.", "content.title", "course_id", false)
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
	RunE:  runCourseModules,
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

// printList fetches a list endpoint and prints using PrintList
func printList(path, emptyMsg, titleKey, idKey string, usePost bool) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	c := client.New(cfg)
	var response map[string]interface{}
	var reqErr error
	if usePost {
		reqErr = c.Post(path, nil, &response)
	} else {
		reqErr = c.Get(path, &response)
	}
	if reqErr != nil {
		return reqErr
	}

	data, ok := response["data"].([]interface{})
	if !ok || len(data) == 0 {
		if output.GetFormat() == output.FormatJSON {
			fmt.Println(`{"data":[]}`)
		} else {
			fmt.Fprintln(os.Stderr, emptyMsg)
		}
		return nil
	}

	items := make([]map[string]interface{}, 0, len(data))
	for _, item := range data {
		if m, ok := item.(map[string]interface{}); ok {
			items = append(items, m)
		}
	}

	return output.PrintList(items, titleKey, idKey)
}

func runCourseModules(cmd *cobra.Command, args []string) error {
	courseID := args[0]

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	// Use teacher endpoint for richer data when teacher auth is available
	if cfg.HasUserAuth() {
		return runCourseModulesTeacher(cfg, courseID)
	}

	// Fall back to user endpoint
	return getJSON("/api/v2/course/user/modules/" + url.PathEscape(courseID))
}

func runCourseModulesTeacher(cfg *config.Config, courseID string) error {
	c := client.New(cfg)

	var resp map[string]interface{}
	reqBody := map[string]string{"course_id": courseID}
	if err := c.Post("/api/v2/course/teacher/course-modules/list", reqBody, &resp); err != nil {
		return err
	}

	modules, ok := resp["data"].([]interface{})
	if !ok || len(modules) == 0 {
		if output.GetFormat() == output.FormatJSON {
			fmt.Println(`{"data":[]}`)
		} else {
			fmt.Fprintln(os.Stderr, "No modules found.")
		}
		return nil
	}

	if output.GetFormat() == output.FormatJSON {
		return output.PrintJSON(resp)
	}

	// Print table header
	fmt.Printf("%-8s %-40s %-12s %5s %7s %10s\n", "Code", "Title", "Status", "SLTs", "Lessons", "Assignment")
	fmt.Printf("%-8s %-40s %-12s %5s %7s %10s\n", "----", "-----", "------", "----", "-------", "----------")

	for _, m := range modules {
		mod, ok := m.(map[string]interface{})
		if !ok {
			continue
		}
		content, ok := mod["content"].(map[string]interface{})
		if !ok {
			continue
		}

		code, _ := content["course_module_code"].(string)
		title, _ := content["title"].(string)
		status, _ := content["module_status"].(string)

		sltCount := 0
		lessonCount := 0
		if slts, ok := content["slts"].([]interface{}); ok {
			sltCount = len(slts)
			for _, slt := range slts {
				sltMap, ok := slt.(map[string]interface{})
				if !ok {
					continue
				}
				if _, hasLesson := sltMap["lesson"].(map[string]interface{}); hasLesson {
					lessonCount++
				}
			}
		}

		hasAssignment := "No"
		if _, ok := content["assignment"].(map[string]interface{}); ok {
			hasAssignment = "Yes"
		}

		// Truncate long titles
		if len(title) > 38 {
			title = title[:35] + "..."
		}

		fmt.Printf("%-8s %-40s %-12s %5d %7d %10s\n", code, title, status, sltCount, lessonCount, hasAssignment)
	}

	return nil
}
