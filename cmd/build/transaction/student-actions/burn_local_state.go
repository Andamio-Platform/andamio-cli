package student_actions

import (
	"github.com/spf13/cobra"
)

var BurnLocalStateCmd = &cobra.Command{
	Use:   "burn-local-state",
	Short: "Build transaction",
	Long:  `Build transactions.`,
	Run: func(cmd *cobra.Command, args []string) {

		// Your access tokens logic here
	},
}
