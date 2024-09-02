package student_actions

import (
	"fmt"

	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/spf13/cobra"
)

var UpdateAssignmentCmd = &cobra.Command{
	Use:   "update-assignment",
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
		if assignmentInfo == "" {
			fmt.Println("Please provide an assignmentInfo using --assignmentInfo flag")
			return
		}
		client.GetUpdateAssignment(userAccessToken, policy, assignmentInfo)
	},
}

func init() {
	UpdateAssignmentCmd.Flags().StringVar(&userAccessToken, "userAccessToken", "", "userAccessToken to check availability for")
	UpdateAssignmentCmd.Flags().StringVar(&policy, "policy", "", "policy to check availability for")
	UpdateAssignmentCmd.Flags().StringVar(&assignmentInfo, "assignmentInfo", "", "assignmentInfo to check availability for")
}
