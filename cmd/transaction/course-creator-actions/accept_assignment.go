package course_creator_actions

import (
	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/spf13/cobra"
)

var (
	userAccessToken string
	studentAlias    string
	policy          string
)

var AcceptAssignmentCmd = &cobra.Command{
	Use:   "accept-assignment",
	Short: "Approve a student commitment to course assignment and issue credential for completion.",
	Long: `
About:
A teacher can accept or deny student commitments to assignments.

This transaction accepts the current assignment for the student with studentAlias in the course specified by policy. 

The transaction must be signed by the holder of userAccessToken.

  `,
	Run: func(cmd *cobra.Command, args []string) {
		client.GetAcceptAssignment(userAccessToken, studentAlias, policy)
	},
}

func init() {
	AcceptAssignmentCmd.Flags().StringVar(&userAccessToken, "userAccessToken", "", "Cardano Asset ID of teacher access token. The wallet holding this asset must sign the generated transaction.")
	AcceptAssignmentCmd.Flags().StringVar(&studentAlias, "studentAlias", "", "Access token name of student with committed assignment")
	AcceptAssignmentCmd.Flags().StringVar(&policy, "policy", "", "Course NFT policy id")

	// Required flags
	AcceptAssignmentCmd.MarkFlagRequired("userAccessToken")
	AcceptAssignmentCmd.MarkFlagRequired("studentAlias")
	AcceptAssignmentCmd.MarkFlagRequired("policy")
}
