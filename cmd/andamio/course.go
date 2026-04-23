package main

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"

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
		return printList(cmd.Context(), "/api/v2/course/user/courses/list", "No courses found.", "content.title", "course_id", false)
	},
}

var courseGetCmd = &cobra.Command{
	Use:   "get <course-id>",
	Short: "Get course details",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return getJSON(cmd.Context(), "/api/v2/course/user/course/get/"+url.PathEscape(args[0]))
	},
}

var courseModulesCmd = &cobra.Command{
	Use:   "modules [course-id]",
	Short: "List modules for a course",
	Long: `List modules for a course.

The course can be specified by ID (positional arg) or by name (--course flag):
  andamio course modules <course-id>
  andamio course modules --course "Intro to Cardano"

Find your course IDs with: andamio teacher courses`,
	Args: cobra.MaximumNArgs(1),
	RunE: runCourseModules,
}

var courseSltsCmd = &cobra.Command{
	Use:   "slts [course-id] <module-code>",
	Short: "List SLTs for a course module",
	Long: `List SLTs for a course module.

The course can be specified by ID (first positional arg) or by name (--course flag):
  andamio course slts <course-id> <module-code>
  andamio course slts <module-code> --course "Intro to Cardano"

Find your course IDs with: andamio teacher courses`,
	Args: cobra.RangeArgs(1, 2),
	RunE: runCourseSlts,
}

var courseLessonCmd = &cobra.Command{
	Use:   "lesson <course-id> <module-code> <slt-index>",
	Short: "Get lesson content by SLT index (integer)",
	Long: `Get lesson content for a specific SLT in a course module.

The slt-index must be a positive integer (e.g., 1, 2, 3), not a code or hash.

Examples:
  andamio course lesson my-course 101 1
  andamio course lesson my-course 101 3`,
	Args: cobra.ExactArgs(3),
	RunE: runCourseLesson,
}

var courseAssignmentCmd = &cobra.Command{
	Use:   "assignment <course-id> <module-code>",
	Short: "Get assignment for a course module",
	Args:  cobra.ExactArgs(2),
	RunE:  runCourseAssignment,
}

var courseIntroCmd = &cobra.Command{
	Use:   "intro <course-id> <module-code>",
	Short: "Get introduction for a course module",
	Args:  cobra.ExactArgs(2),
	RunE:  runCourseIntro,
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

	// --course flag for name-based course resolution
	courseModulesCmd.Flags().String("course", "", "Course name or substring (alternative to course-id arg)")
	courseSltsCmd.Flags().String("course", "", "Course name or substring (alternative to course-id arg)")
}

// teacherCourse holds the fields needed from the teacher courses list
type teacherCourse struct {
	CourseID string
	Title    string
}

// fetchTeacherCourses calls POST /v2/course/teacher/courses/list and returns parsed courses
func fetchTeacherCourses(ctx context.Context, c *client.Client) ([]teacherCourse, error) {
	var resp map[string]interface{}
	if err := c.Post(ctx, "/api/v2/course/teacher/courses/list", nil, &resp); err != nil {
		return nil, fmt.Errorf("failed to list teacher courses: %w", err)
	}

	data, ok := resp["data"].([]interface{})
	if !ok || len(data) == 0 {
		return nil, nil
	}

	courses := make([]teacherCourse, 0, len(data))
	for _, item := range data {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		tc := teacherCourse{}
		tc.CourseID, _ = m["course_id"].(string)
		if content, ok := m["content"].(map[string]interface{}); ok {
			tc.Title, _ = content["title"].(string)
		}
		if tc.CourseID != "" {
			courses = append(courses, tc)
		}
	}
	return courses, nil
}

// resolveCourseID resolves a course ID from a positional arg or --course flag.
// If courseIDArg is non-empty, it is used directly. Otherwise, --course is
// used for substring matching against the teacher courses list.
func resolveCourseID(ctx context.Context, c *client.Client, courseIDArg string, cmd *cobra.Command) (string, error) {
	// If positional arg is available, use it directly
	if courseIDArg != "" {
		return courseIDArg, nil
	}

	// Check --course flag
	courseName, _ := cmd.Flags().GetString("course")
	if courseName == "" {
		return "", fmt.Errorf(
			"course-id required\n\nList your courses with:\n  andamio teacher courses\n  andamio teacher courses --output json",
		)
	}

	// Fetch teacher courses and match by title substring
	courses, err := fetchTeacherCourses(ctx, c)
	if err != nil {
		return "", err
	}
	if len(courses) == 0 {
		return "", fmt.Errorf("no teacher courses found")
	}

	var matches []teacherCourse
	needle := strings.ToLower(courseName)
	for _, course := range courses {
		if strings.Contains(strings.ToLower(course.Title), needle) {
			matches = append(matches, course)
		}
	}

	switch len(matches) {
	case 0:
		return "", fmt.Errorf("no course matching %q found\n\nList your courses:\n  andamio teacher courses", courseName)
	case 1:
		return matches[0].CourseID, nil
	default:
		var lines []string
		for _, m := range matches {
			lines = append(lines, fmt.Sprintf("  %s  %s", m.CourseID, m.Title))
		}
		return "", fmt.Errorf(
			"%q matches multiple courses:\n%s\n\nUse a more specific name or pass the course-id directly.",
			courseName, strings.Join(lines, "\n"),
		)
	}
}

// resolveCourseIDFromFlags resolves a course ID from --course-id or --course flag.
// Used by import commands where both flags are available.
func resolveCourseIDFromFlags(ctx context.Context, cmd *cobra.Command) (string, error) {
	courseID, _ := cmd.Flags().GetString("course-id")
	if courseID != "" {
		return courseID, nil
	}
	cfg, err := config.Load()
	if err != nil {
		return "", err
	}
	c := client.New(cfg)
	return resolveCourseID(ctx, c, "", cmd)
}

func runCourseModules(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	c := client.New(cfg)

	var courseIDArg string
	if len(args) > 0 {
		courseIDArg = args[0]
	}
	courseID, err := resolveCourseID(ctx, c, courseIDArg, cmd)
	if err != nil {
		return err
	}

	// Use teacher endpoint for richer data when teacher auth is available
	if cfg.HasUserAuth() {
		return runCourseModulesTeacher(ctx, cfg, courseID)
	}

	// Fall back to user endpoint
	return getJSON(ctx, "/api/v2/course/user/modules/"+url.PathEscape(courseID))
}

func runCourseModulesTeacher(ctx context.Context, cfg *config.Config, courseID string) error {
	c := client.New(cfg)

	var resp map[string]interface{}
	reqBody := map[string]string{"course_id": courseID}
	if err := c.Post(ctx, "/api/v2/course/teacher/course-modules/list", reqBody, &resp); err != nil {
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

		title = truncateUTF8(title, 38)

		fmt.Printf("%-8s %-40s %-12s %5d %7d %10s\n", code, title, status, sltCount, lessonCount, hasAssignment)
	}

	return nil
}

func runCourseSlts(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	c := client.New(cfg)

	var courseID, moduleCode string
	if len(args) == 2 {
		// slts <course-id> <module-code>
		courseID = args[0]
		moduleCode = args[1]
	} else {
		// slts <module-code> --course "Name"
		moduleCode = args[0]
		courseID, err = resolveCourseID(ctx, c, "", cmd)
		if err != nil {
			return err
		}
	}

	// Use teacher endpoint for lesson presence data when JWT is available
	if cfg.HasUserAuth() {
		return runCourseSltsTeacher(ctx, cfg, courseID, moduleCode)
	}

	// Fall back to user endpoint (raw JSON, no lesson presence info)
	return getJSON(ctx, "/api/v2/course/user/slts/"+url.PathEscape(courseID)+"/"+url.PathEscape(moduleCode))
}

// fetchTeacherModuleContent fetches the content map for a specific module via the teacher endpoint.
// Returns the raw "content" object from the matching module, or an error if not found.
// This is used by course slts, lesson, intro, and assignment commands to access draft module data.
func fetchTeacherModuleContent(ctx context.Context, cfg *config.Config, courseID, moduleCode string) (map[string]interface{}, error) {
	c := client.New(cfg)

	var resp map[string]interface{}
	reqBody := map[string]string{"course_id": courseID}
	if err := c.Post(ctx, "/api/v2/course/teacher/course-modules/list", reqBody, &resp); err != nil {
		return nil, err
	}

	modules, ok := resp["data"].([]interface{})
	if !ok || len(modules) == 0 {
		return nil, &apierr.NotFoundError{
			Message: fmt.Sprintf("Module %s not found. Run 'andamio course modules %s' to see available modules.", moduleCode, courseID),
		}
	}

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
			return content, nil
		}
	}

	return nil, &apierr.NotFoundError{
		Message: fmt.Sprintf("Module %s not found. Run 'andamio course modules %s' to see available modules.", moduleCode, courseID),
	}
}

func runCourseSltsTeacher(ctx context.Context, cfg *config.Config, courseID, moduleCode string) error {
	content, err := fetchTeacherModuleContent(ctx, cfg, courseID, moduleCode)
	if err != nil {
		return err
	}

	targetSlts, _ := content["slts"].([]interface{})

	if len(targetSlts) == 0 {
		if output.GetFormat() == output.FormatJSON {
			return output.PrintJSON(map[string]interface{}{"data": []interface{}{}})
		}
		fmt.Fprintln(os.Stderr, "No SLTs found for this module.")
		return nil
	}

	// Build structured items for all output formats
	items := make([]map[string]interface{}, 0, len(targetSlts))
	for _, slt := range targetSlts {
		sltMap, ok := slt.(map[string]interface{})
		if !ok {
			continue
		}

		hasLesson := "No"
		if _, ok := sltMap["lesson"].(map[string]interface{}); ok {
			hasLesson = "Yes"
		}

		items = append(items, map[string]interface{}{
			"slt_index":  fmt.Sprintf("%v", sltMap["slt_index"]),
			"slt_text":   sltMap["slt_text"],
			"has_lesson": hasLesson,
		})
	}

	if output.GetFormat() != output.FormatText {
		return output.PrintJSON(map[string]interface{}{"data": items})
	}

	// Text mode: formatted table with truncated SLT text
	fmt.Printf("%-7s %-50s %s\n", "INDEX", "SLT TEXT", "HAS LESSON")
	fmt.Printf("%-7s %-50s %s\n", "-----", "--------", "----------")

	for _, item := range items {
		text, _ := item["slt_text"].(string)
		text = truncateUTF8(text, 50)
		fmt.Printf("%-7s %-50s %s\n", item["slt_index"], text, item["has_lesson"])
	}

	return nil
}

func runCourseLesson(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	courseID, moduleCode, sltIndex := args[0], args[1], args[2]

	// Try teacher endpoint first for draft module support
	cfg, err := config.Load()
	if err == nil && cfg.HasUserAuth() {
		content, terr := fetchTeacherModuleContent(ctx, cfg, courseID, moduleCode)
		if terr == nil {
			slts, _ := content["slts"].([]interface{})
			for _, slt := range slts {
				sltMap, ok := slt.(map[string]interface{})
				if !ok {
					continue
				}
				idx := fmt.Sprintf("%v", sltMap["slt_index"])
				if idx == sltIndex {
					lesson, ok := sltMap["lesson"].(map[string]interface{})
					if ok {
						return output.PrintJSON(map[string]interface{}{"data": lesson})
					}
					// SLT exists but has no lesson content
					return &apierr.NotFoundError{
						Message: fmt.Sprintf("SLT %s exists in module %s but has no lesson content. Import lesson content with 'andamio course import'.",
							sltIndex, moduleCode),
					}
				}
			}
		} else {
			var notFound *apierr.NotFoundError
			if !errors.As(terr, &notFound) {
				// Propagate non-NotFound errors (auth, network, server errors)
				return terr
			}
		}
	}

	// Fall back to user endpoint (no JWT, or module not found via teacher)
	path := "/api/v2/course/user/lesson/" + url.PathEscape(courseID) + "/" + url.PathEscape(moduleCode) + "/" + url.PathEscape(sltIndex)
	hint := fmt.Sprintf("No lesson found for SLT %s in module %s. Run 'andamio course slts %s %s' to see which SLTs have lessons.",
		sltIndex, moduleCode, courseID, moduleCode)
	return getJSONWithHint(ctx, path, hint)
}

func runCourseIntro(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	courseID, moduleCode := args[0], args[1]

	// Try teacher endpoint first for draft module support
	cfg, err := config.Load()
	if err == nil && cfg.HasUserAuth() {
		content, terr := fetchTeacherModuleContent(ctx, cfg, courseID, moduleCode)
		if terr == nil {
			if intro, ok := content["introduction"].(map[string]interface{}); ok {
				return output.PrintJSON(map[string]interface{}{"data": intro})
			}
			// Module exists but has no introduction
			return &apierr.NotFoundError{
				Message: fmt.Sprintf("Module %s has no introduction content. Import content with 'andamio course import'.",
					moduleCode),
			}
		} else {
			var notFound *apierr.NotFoundError
			if !errors.As(terr, &notFound) {
				return terr
			}
		}
	}

	// Fall back to user endpoint (no JWT, or module not found via teacher)
	path := "/api/v2/course/user/introduction/" + url.PathEscape(courseID) + "/" + url.PathEscape(moduleCode)
	hint := fmt.Sprintf("No introduction found for module %s. Run 'andamio course modules %s' to see available modules.",
		moduleCode, courseID)
	return getJSONWithHint(ctx, path, hint)
}

func runCourseAssignment(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	courseID, moduleCode := args[0], args[1]

	// Try teacher endpoint first for draft module support
	cfg, err := config.Load()
	if err == nil && cfg.HasUserAuth() {
		content, terr := fetchTeacherModuleContent(ctx, cfg, courseID, moduleCode)
		if terr == nil {
			if assign, ok := content["assignment"].(map[string]interface{}); ok {
				return output.PrintJSON(map[string]interface{}{"data": assign})
			}
			// Module exists but has no assignment
			return &apierr.NotFoundError{
				Message: fmt.Sprintf("Module %s has no assignment content. Import content with 'andamio course import'.",
					moduleCode),
			}
		} else {
			var notFound *apierr.NotFoundError
			if !errors.As(terr, &notFound) {
				return terr
			}
		}
	}

	// Fall back to user endpoint (no JWT, or module not found via teacher)
	path := "/api/v2/course/user/assignment/" + url.PathEscape(courseID) + "/" + url.PathEscape(moduleCode)
	hint := fmt.Sprintf("No assignment found for module %s. Run 'andamio course modules %s' to see which modules have assignments.",
		moduleCode, courseID)
	return getJSONWithHint(ctx, path, hint)
}
