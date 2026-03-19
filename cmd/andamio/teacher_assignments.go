package main

import (
	"fmt"
	"os"

	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/Andamio-Platform/andamio-cli/internal/config"
	"github.com/Andamio-Platform/andamio-cli/internal/output"
	"github.com/spf13/cobra"
)

var teacherAssignmentsCmd = &cobra.Command{
	Use:   "assignments",
	Short: "Manage assignment reviews (teacher role)",
}

var teacherAssignmentsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List pending assignment commitments for review",
	Long: `List assignment commitments pending teacher review.

Without --course, returns a lightweight summary across all courses.
With --course, returns full merged history (on-chain + DB) for that course.

Examples:
  andamio teacher assignments list
  andamio teacher assignments list --course <course-id>
  andamio teacher assignments list --course <course-id> --output json`,
	RunE: runTeacherAssignmentsList,
}

var teacherAssignmentsGetCmd = &cobra.Command{
	Use:   "get <course-id> <module-code> <student-alias>",
	Short: "Get a specific assignment commitment for review",
	Long: `Get full details for a specific student's assignment commitment.

Examples:
  andamio teacher assignments get <course-id> <module-code> <student-alias>
  andamio teacher assignments get <course-id> <module-code> <student-alias> --output json`,
	Args: cobra.ExactArgs(3),
	RunE: runTeacherAssignmentsGet,
}

func init() {
	teacherCmd.AddCommand(teacherAssignmentsCmd)
	teacherAssignmentsCmd.AddCommand(teacherAssignmentsListCmd)
	teacherAssignmentsCmd.AddCommand(teacherAssignmentsGetCmd)

	// List flags (all optional)
	teacherAssignmentsListCmd.Flags().String("course", "", "Filter by course ID")
}

func runTeacherAssignmentsList(cmd *cobra.Command, args []string) error {
	courseID, _ := cmd.Flags().GetString("course")

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	c := client.New(cfg)

	// Build request body with optional course filter
	var body interface{}
	if courseID != "" {
		body = map[string]string{"course_id": courseID}
	}

	var resp map[string]interface{}
	if err := c.Post("/api/v2/course/teacher/assignment-commitments/list", body, &resp); err != nil {
		return err
	}

	if output.GetFormat() == output.FormatJSON {
		return output.PrintJSON(resp)
	}

	data, ok := resp["data"].([]interface{})
	if !ok || len(data) == 0 {
		fmt.Fprintln(os.Stderr, "No pending assignment commitments found.")
		return nil
	}

	// Print formatted table
	fmt.Printf("%-20s %-12s %-15s %s\n", "STUDENT", "MODULE", "SOURCE", "COURSE ID")
	fmt.Printf("%-20s %-12s %-15s %s\n", "-------", "------", "------", "---------")

	for _, item := range data {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		student, _ := m["student_alias"].(string)
		moduleCode, _ := m["course_module_code"].(string)
		source, _ := m["source"].(string)
		cID, _ := m["course_id"].(string)

		student = truncateUTF8(student, 20)

		fmt.Printf("%-20s %-12s %-15s %s\n", student, moduleCode, source, cID)
	}

	return nil
}

func runTeacherAssignmentsGet(cmd *cobra.Command, args []string) error {
	courseID, moduleCode, studentAlias := args[0], args[1], args[2]

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	c := client.New(cfg)

	// Fetch full commitment data for this course, then filter by module + student
	body := map[string]string{"course_id": courseID}
	var resp map[string]interface{}
	if err := c.Post("/api/v2/course/teacher/assignment-commitments/list", body, &resp); err != nil {
		return err
	}

	data, ok := resp["data"].([]interface{})
	if !ok {
		return fmt.Errorf("no commitments found for course %s", courseID)
	}

	for _, item := range data {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		mCode, _ := m["course_module_code"].(string)
		alias, _ := m["student_alias"].(string)
		if mCode == moduleCode && alias == studentAlias {
			return output.PrintJSON(m)
		}
	}

	return fmt.Errorf("no commitment found for student %q in module %s. Run 'andamio teacher assignments list --course %s' to see pending commitments",
		studentAlias, moduleCode, courseID)
}

