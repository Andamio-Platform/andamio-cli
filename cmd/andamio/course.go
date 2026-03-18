package main

import (
	"errors"
	"fmt"
	"net/url"
	"os"

	"github.com/Andamio-Platform/andamio-cli/internal/apierr"
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
	RunE:  runCourseSlts,
}

var courseLessonCmd = &cobra.Command{
	Use:   "lesson <course-id> <module-code> <slt-index>",
	Short: "Get lesson content",
	Args:  cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		courseID, moduleCode, sltIndex := args[0], args[1], args[2]
		err := getJSON("/api/v2/course/user/lesson/" + url.PathEscape(courseID) + "/" + url.PathEscape(moduleCode) + "/" + url.PathEscape(sltIndex))
		if err != nil {
			var notFound *apierr.NotFoundError
			if errors.As(err, &notFound) {
				return &apierr.NotFoundError{
					Message: fmt.Sprintf("No lesson found for SLT %s in module %s. Run 'andamio course slts %s %s' to see which SLTs have lessons.",
						sltIndex, moduleCode, courseID, moduleCode),
				}
			}
			return err
		}
		return nil
	},
}

var courseAssignmentCmd = &cobra.Command{
	Use:   "assignment <course-id> <module-code>",
	Short: "Get assignment for a course module",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		courseID, moduleCode := args[0], args[1]
		err := getJSON("/api/v2/course/user/assignment/" + url.PathEscape(courseID) + "/" + url.PathEscape(moduleCode))
		if err != nil {
			var notFound *apierr.NotFoundError
			if errors.As(err, &notFound) {
				return &apierr.NotFoundError{
					Message: fmt.Sprintf("No assignment found for module %s. Run 'andamio course modules %s' to see which modules have assignments.",
						moduleCode, courseID),
				}
			}
			return err
		}
		return nil
	},
}

var courseIntroCmd = &cobra.Command{
	Use:   "intro <course-id> <module-code>",
	Short: "Get introduction for a course module",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		courseID, moduleCode := args[0], args[1]
		err := getJSON("/api/v2/course/user/introduction/" + url.PathEscape(courseID) + "/" + url.PathEscape(moduleCode))
		if err != nil {
			var notFound *apierr.NotFoundError
			if errors.As(err, &notFound) {
				return &apierr.NotFoundError{
					Message: fmt.Sprintf("No introduction found for module %s. Run 'andamio course modules %s' to see available modules.",
						moduleCode, courseID),
				}
			}
			return err
		}
		return nil
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
			return output.PrintJSON(map[string]interface{}{"data": []interface{}{}})
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
			return output.PrintJSON(map[string]interface{}{"data": []interface{}{}})
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

func runCourseSlts(cmd *cobra.Command, args []string) error {
	courseID := args[0]
	moduleCode := args[1]

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	// Use teacher endpoint for lesson presence data when JWT is available
	if cfg.HasUserAuth() {
		return runCourseSltsTeacher(cfg, courseID, moduleCode)
	}

	// Fall back to user endpoint (raw JSON, no lesson presence info)
	return getJSON("/api/v2/course/user/slts/" + url.PathEscape(courseID) + "/" + url.PathEscape(moduleCode))
}

func runCourseSltsTeacher(cfg *config.Config, courseID, moduleCode string) error {
	c := client.New(cfg)

	var resp map[string]interface{}
	reqBody := map[string]string{"course_id": courseID}
	if err := c.Post("/api/v2/course/teacher/course-modules/list", reqBody, &resp); err != nil {
		return err
	}

	modules, ok := resp["data"].([]interface{})
	if !ok || len(modules) == 0 {
		if output.GetFormat() == output.FormatJSON {
			return output.PrintJSON(map[string]interface{}{"data": []interface{}{}})
		}
		fmt.Fprintln(os.Stderr, "No modules found.")
		return nil
	}

	// Find the matching module
	var targetSlts []interface{}
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
		if code == moduleCode {
			targetSlts, _ = content["slts"].([]interface{})
			break
		}
	}

	if targetSlts == nil {
		return &apierr.NotFoundError{
			Message: fmt.Sprintf("Module %s not found. Run 'andamio course modules %s' to see available modules.", moduleCode, courseID),
		}
	}

	if len(targetSlts) == 0 {
		if output.GetFormat() == output.FormatJSON {
			return output.PrintJSON(map[string]interface{}{"data": []interface{}{}})
		}
		fmt.Fprintln(os.Stderr, "No SLTs found for this module.")
		return nil
	}

	if output.GetFormat() == output.FormatJSON {
		return output.PrintJSON(map[string]interface{}{"data": targetSlts})
	}

	// Print formatted table
	fmt.Printf("%-7s %-50s %s\n", "INDEX", "SLT TEXT", "HAS LESSON")
	fmt.Printf("%-7s %-50s %s\n", "-----", "--------", "----------")

	for _, slt := range targetSlts {
		sltMap, ok := slt.(map[string]interface{})
		if !ok {
			continue
		}

		index := fmt.Sprintf("%v", sltMap["slt_index"])
		text, _ := sltMap["slt_text"].(string)
		if len(text) > 48 {
			text = text[:45] + "..."
		}

		hasLesson := "No"
		if _, ok := sltMap["lesson"].(map[string]interface{}); ok {
			hasLesson = "Yes"
		}

		fmt.Printf("%-7s %-50s %s\n", index, text, hasLesson)
	}

	return nil
}
