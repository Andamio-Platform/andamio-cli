package student_actions

import (
	"fmt"

	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/spf13/cobra"
)

var LeaveAssignmentCmd = &cobra.Command{
	Use:   "leave-assignment",
	Short: "Build transaction",
	Long:  `Build transactions.`,
	Run: func(cmd *cobra.Command, args []string) {
		if userAccessToken == "" {
			fmt.Println("Please provide an userAccessToken using --userAccessToken flag")
			return
		}
		if policy == "" {
			fmt.Println("Please provide an policy using --policy flag")
			return
		}
		client.GetLeaveAssignment(userAccessToken, policy)
	},
}

func init() {
	LeaveAssignmentCmd.Flags().StringVar(&userAccessToken, "userAccessToken", "", "userAccessToken to check availability for")
	LeaveAssignmentCmd.Flags().StringVar(&policy, "policy", "", "policy to check availability for")
}
