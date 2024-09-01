package query

import (
	"fmt"
	"os"

	"github.com/Andamio-Platform/andamio-cli/cmd/query/course"
	"github.com/spf13/cobra"
)

var CourseCmd = &cobra.Command{
	Use:   "course",
	Short: "change this",
	Long:  `change this.`,
	Run: func(cmd *cobra.Command, args []string) {
		// If no arguments are passed, show help
		if len(args) == 0 {
			cmd.Help()
			return
		}

		// If an invalid subcommand is passed, show an error message
		fmt.Printf("Error: '%s' is not a valid subcommand for 'network'\n", args[0])
		fmt.Println("Run './andamio-cli query network --help' for available subcommands.")
		os.Exit(1) // Exit with a non-zero status to indicate an error
	},
}

func init() {
	CourseCmd.AddCommand(course.AssignmentValidatorCmd)
	CourseCmd.AddCommand(course.CourseGovernanceValidatorCmd)
	CourseCmd.AddCommand(course.CourseStateCmd)
	CourseCmd.AddCommand(course.ModuleRefValidatorCmd)
}
