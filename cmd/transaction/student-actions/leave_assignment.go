package student_actions

import (
	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/spf13/cobra"
)

var LeaveAssignmentCmd = &cobra.Command{
	Use:   "leave-assignment",
	Short: "Cancel assignment commitment",
	Long: `
About:
A student can cancel a commitment to an assignment any time.  

This transaction cancels the current commitment of userAccessToken in the course specified by policy.

The transaction must be signed by the holder of userAccessToken.

Example:
  andamio-cli transaction student leave-assignment \ 
    --userAccessToken ASSET_ID (POLICY_ID+ASSET_NAME) \
    --policy POLICY_ID


`,
	Run: func(cmd *cobra.Command, args []string) {
		client.GetLeaveAssignment(userAccessToken, policy)
	},
}

func init() {
	LeaveAssignmentCmd.Flags().StringVar(&userAccessToken, "userAccessToken", "", "Cardano Asset ID of student access token. The wallet holding this asset must sign the generated transaction.")
	LeaveAssignmentCmd.Flags().StringVar(&policy, "policy", "", "Course NFT policy id")

	// Required flags
	LeaveAssignmentCmd.MarkFlagRequired("userAccessToken")
	LeaveAssignmentCmd.MarkFlagRequired("policy")
}
