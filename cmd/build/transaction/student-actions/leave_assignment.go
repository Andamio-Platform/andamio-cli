package student_actions

import (
	"github.com/spf13/cobra"
)

var LeaveAssignmentCmd = &cobra.Command{
	Use:   "leave-assignment",
	Short: "Build transaction",
	Long:  `Build transactions.`,
	Run: func(cmd *cobra.Command, args []string) {

		// Your access tokens logic here
	},
}
