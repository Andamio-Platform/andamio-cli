package main

import (
	"github.com/Andamio-Platform/andamio-cli/internal/apierr"
	"github.com/Andamio-Platform/andamio-cli/internal/config"
	"github.com/spf13/cobra"
)

var teacherCmd = &cobra.Command{
	Use:   "teacher",
	Short: "Teacher operations (requires user login)",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Chain with root's PersistentPreRunE (output format)
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
