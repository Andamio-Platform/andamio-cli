package course_creator_actions

import (
	"github.com/spf13/cobra"
)

var AcceptAssignmentCmd = &cobra.Command{
	Use:   "accept-assignment",
	Short: "Build transaction",
	Long:  `Build transactions.`,
	Run: func(cmd *cobra.Command, args []string) {

		// Your access tokens logic here
	},
}
