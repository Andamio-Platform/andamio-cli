package student_actions

import (
	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/spf13/cobra"
)

var UpdateAssignmentCmd = &cobra.Command{
	Use:   "update-assignment",
	Short: "Update assginment evidence",
	Long: `
About:
A student can update assignment info any time.  

This transaction allows the holder of userAccessToken to update assignmentInfo in the course specified by policy.

The transaction must be signed by the holder of userAccessToken.

`,
	Run: func(cmd *cobra.Command, args []string) {
		client.GetUpdateAssignment(userAccessToken, policy, assignmentInfo)
	},
}

func init() {
	UpdateAssignmentCmd.Flags().StringVar(&userAccessToken, "userAccessToken", "", "Cardano Asset ID of student access token. The wallet holding this asset must sign the generated transaction.")
	UpdateAssignmentCmd.Flags().StringVar(&policy, "policy", "", "Course NFT policy id")
	UpdateAssignmentCmd.Flags().StringVar(&assignmentInfo, "assignmentInfo", "", "Evidence of assignment completion")

	// Required flags
	UpdateAssignmentCmd.MarkFlagRequired("userAccessToken")
	UpdateAssignmentCmd.MarkFlagRequired("policy")
	UpdateAssignmentCmd.MarkFlagRequired("assignmentInfo")
}
