package course_governance_validator

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var CourseGovernanceValidatorCmd = &cobra.Command{
	Use:   "course-governance-validator",
	Short: "View network course governance data",
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
	CourseGovernanceValidatorCmd.AddCommand(AllCourseGovernanceValidatorUtxosCmd)
	CourseGovernanceValidatorCmd.AddCommand(AllDecodedCourseGovDatumsCmd)
	CourseGovernanceValidatorCmd.AddCommand(CourseGovernanceValidatorUtxoCmd)
	CourseGovernanceValidatorCmd.AddCommand(CoursePoliciesCmd)
}
