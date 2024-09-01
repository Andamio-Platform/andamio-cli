package transaction

import (
	"fmt"
	"os"

	student_actions "github.com/Andamio-Platform/andamio-cli/cmd/build/transaction/student-actions"
	"github.com/spf13/cobra"
)

var StudentActionsCmd = &cobra.Command{
	Use:   "student-actions",
	Short: "Build transaction",
	Long:  `Build transactions.`,
	Run: func(cmd *cobra.Command, args []string) {

		// If no arguments are passed, show help
		if len(args) == 0 {
			cmd.Help()
			return
		}

		// If an invalid subcommand is passed, show an error message
		fmt.Printf("Error: '%s' is not a valid subcommand for 'build'\n", args[0])
		fmt.Println("Run './andamio-cli build --help' for available subcommands.")
		os.Exit(1) // Exit with a non-zero status to indicate an error
	},
}

func init() {
	StudentActionsCmd.AddCommand(student_actions.MintLocalStateCmd)
	StudentActionsCmd.AddCommand(student_actions.CommitToAssignmentCmd)
	StudentActionsCmd.AddCommand(student_actions.UpdateAssignmentCmd)
	StudentActionsCmd.AddCommand(student_actions.LeaveAssignmentCmd)
	StudentActionsCmd.AddCommand(student_actions.BurnLocalStateCmd)
}
