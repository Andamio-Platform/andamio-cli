package student_actions

import (
	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/spf13/cobra"
)

var (
	assignmentCode string
	assignmentInfo string
)

var CommitToAssignmentCmd = &cobra.Command{
	Use:   "commit-to-assignment",
	Short: "Commit to an assignment",
	Long: `
About:
When a student is enrolled in a course, they can commit to assignments and earn credentials.

This transaction commits userAccessToken to assignmentCode in the course specified by policy.

To make a commitment, the student must provide assignmentInfo as evidence.

The transaction must be signed by the holder of userAccessToken.

To view valid assigmentCodes, use andamio-cli query course module decoded-module-ref-datums 

Example:
  andamio-cli transaction student commit-to-assignment \ 
    --userAccessToken ASSET_ID (POLICY_ID+ASSET_NAME) \
    --policy POLICY_ID \
    --assignmentCode STRING \
    --assignmentInfo STRING


`,
	Run: func(cmd *cobra.Command, args []string) {
		client.GetCommitToAssignment(userAccessToken, policy, assignmentCode, assignmentInfo)
	},
}

func init() {
	CommitToAssignmentCmd.Flags().StringVar(&userAccessToken, "userAccessToken", "", "Cardano Asset ID of student access token. The wallet holding this asset must sign the generated transaction.")
	CommitToAssignmentCmd.Flags().StringVar(&policy, "policy", "", "Course NFT policy id")
	CommitToAssignmentCmd.Flags().StringVar(&assignmentCode, "assignmentCode", "", "Identifier for Assignment, corresponding to the asset name of a course module token.")
	CommitToAssignmentCmd.Flags().StringVar(&assignmentInfo, "assignmentInfo", "", "Evidence of assignment completion")

	// Required flags
	CommitToAssignmentCmd.MarkFlagRequired("userAccessToken")
	CommitToAssignmentCmd.MarkFlagRequired("policy")
	CommitToAssignmentCmd.MarkFlagRequired("assignmentCode")
	CommitToAssignmentCmd.MarkFlagRequired("assignmentInfo")
}
