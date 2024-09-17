package course_creator_actions

import (
	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/spf13/cobra"
)

var DenyAssignmentCmd = &cobra.Command{
	Use:   "deny-assignment",
	Short: "Deny a student commitment to course assignment",
	Long: `
About:
A teacher can accept or deny student commitments to assignments.

This transaction denies the current assignment for the student with studentAlias in the course specified by policy. 

The transaction must be signed by the holder of userAccessToken.

Example:
  andamio-cli transaction course-creator deny-assignment \ 
    --userAccessToken ASSET_ID (POLICY_ID+ASSET_NAME) \
    --studentAlias STRING \
    --policy POLICY_ID


  `,
	Run: func(cmd *cobra.Command, args []string) {
		client.GetDenyAssignment(userAccessToken, studentAlias, policy)
	},
}

func init() {
	DenyAssignmentCmd.Flags().StringVar(&userAccessToken, "userAccessToken", "", "Cardano Asset ID of teacher access token. The wallet holding this asset must sign the generated transaction.")
	DenyAssignmentCmd.Flags().StringVar(&studentAlias, "studentAlias", "", "Access token name of student with committed assignment")
	DenyAssignmentCmd.Flags().StringVar(&policy, "policy", "", "Course NFT policy id")

	// Required flags
	DenyAssignmentCmd.MarkFlagRequired("userAccessToken")
	DenyAssignmentCmd.MarkFlagRequired("studentAlias")
	DenyAssignmentCmd.MarkFlagRequired("policy")
}
