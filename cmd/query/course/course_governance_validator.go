package course

import (
	"fmt"
	"os"

	course_governance_validator "github.com/Andamio-Platform/andamio-cli/cmd/query/course/course-governance-validator"
	"github.com/spf13/cobra"
)

var CourseGovernanceValidatorCmd = &cobra.Command{
	Use:   "course-governance-validator",
	Short: "",
	Long:  `.`,
	Run: func(cmd *cobra.Command, args []string) {

		// If no arguments are passed, show help
		if len(args) == 0 {
			cmd.Help()
			return
		}

		// If an invalid subcommand is passed, show an error message
		fmt.Printf("Error: '%s' is not a valid subcommand for 'course-governance-validator'\n", args[0])
		fmt.Println("Run './andamio-cli query course course-governance-validator --help' for available subcommands.")
		os.Exit(1) // Exit with a non-zero status to indicate an error
	},
}

func init() {
	CourseGovernanceValidatorCmd.AddCommand(course_governance_validator.AllCourseGovernanceValidatorUtxosCmd)
	CourseGovernanceValidatorCmd.AddCommand(course_governance_validator.AllDecodedCourseGovDatumsCmd)
	CourseGovernanceValidatorCmd.AddCommand(course_governance_validator.CourseGovernanceValidatorUtxoCmd)
	CourseGovernanceValidatorCmd.AddCommand(course_governance_validator.CoursePoliciesCmd)
}
