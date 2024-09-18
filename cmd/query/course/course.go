package course

import (
	"fmt"
	"os"

	assignment_validator "github.com/Andamio-Platform/andamio-cli/cmd/query/course/assignment-validator"
	course_governance_validator "github.com/Andamio-Platform/andamio-cli/cmd/query/course/course-governance-validator"
	course_state "github.com/Andamio-Platform/andamio-cli/cmd/query/course/course-state"
	module_ref_validator "github.com/Andamio-Platform/andamio-cli/cmd/query/course/modules"
	"github.com/spf13/cobra"
)

var CourseCmd = &cobra.Command{
	Use:   "course",
	Short: "View course details",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		// If no arguments are passed, show help
		if len(args) == 0 {
			cmd.Help()
			return
		}

		// If an invalid subcommand is passed, show an error message
		fmt.Printf("Error: '%s' is not a valid subcommand for 'course'\n", args[0])
		fmt.Println("Run './andamio-cli query course --help' for available subcommands.")
		os.Exit(1) // Exit with a non-zero status to indicate an error
	},
}

func init() {
	CourseCmd.AddCommand(assignment_validator.AssignmentValidatorCmd)
	CourseCmd.AddCommand(course_governance_validator.CourseGovernanceValidatorCmd)
	CourseCmd.AddCommand(course_state.CourseStateCmd)
	CourseCmd.AddCommand(module_ref_validator.ModuleRefValidatorCmd)
}
