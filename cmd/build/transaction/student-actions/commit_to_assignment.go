package student_actions

import (
	"fmt"

	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/spf13/cobra"
)

var assignmentCode string
var assignmentInfo string

var CommitToAssignmentCmd = &cobra.Command{
	Use:   "commit-to-assignment",
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
		if assignmentCode == "" {
			fmt.Println("Please provide an assignmentCode using --assignmentCode flag")
			return
		}
		if assignmentInfo == "" {
			fmt.Println("Please provide an assignmentInfo using --assignmentInfo flag")
			return
		}
		client.GetCommitToAssignment(userAccessToken, policy, assignmentCode, assignmentInfo)
	},
}

func init() {
	CommitToAssignmentCmd.Flags().StringVar(&userAccessToken, "userAccessToken", "", "")
	CommitToAssignmentCmd.Flags().StringVar(&policy, "policy", "", "")
	CommitToAssignmentCmd.Flags().StringVar(&assignmentCode, "assignmentCode", "", "")
	CommitToAssignmentCmd.Flags().StringVar(&assignmentInfo, "assignmentInfo", "", "")
}
