package main

import "github.com/spf13/cobra"

var teacherCmd = &cobra.Command{
	Use:               "teacher",
	Short:             "Teacher operations (requires user login)",
	PersistentPreRunE: jwtAuthPreRunE,
}

var teacherCoursesCmd = &cobra.Command{
	Use:   "courses",
	Short: "List courses where you are a teacher",
	RunE: func(cmd *cobra.Command, args []string) error {
		return printList(
			"/api/v2/course/teacher/courses/list",
			"No courses found where you are a teacher.",
			"content.title", "course_id", true,
		)
	},
}

func init() {
	rootCmd.AddCommand(teacherCmd)
	teacherCmd.AddCommand(teacherCoursesCmd)
}
