package transaction

import (
	"fmt"
	"os"

	course_creator_actions "github.com/Andamio-Platform/andamio-cli/cmd/transaction/course-creator-actions"
	"github.com/spf13/cobra"
)

var CourseCreatorActionsCmd = &cobra.Command{
	Use:   "course-creator",
	Short: "Transactions for course creators",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		// If no arguments are passed, show help
		if len(args) == 0 {
			cmd.Help()
			return
		}

		// If an invalid subcommand is passed, show an error message
		fmt.Printf("Error: '%s' is not a valid subcommand for 'course-creator-actions'\n", args[0])
		fmt.Println("Run './andamio-cli build transaction course-creator-actions --help' for available subcommands.")
		os.Exit(1) // Exit with a non-zero status to indicate an error
	},
}

func init() {
	CourseCreatorActionsCmd.AddCommand(course_creator_actions.MintModuleTokensCmd)
	CourseCreatorActionsCmd.AddCommand(course_creator_actions.AcceptAssignmentCmd)
	CourseCreatorActionsCmd.AddCommand(course_creator_actions.DenyAssignmentCmd)
}
