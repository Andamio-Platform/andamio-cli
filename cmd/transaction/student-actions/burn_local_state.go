package student_actions

import (
	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/spf13/cobra"
)

var (
	userAccessToken string
	policy          string
)

var BurnLocalStateCmd = &cobra.Command{
	Use:   "burn-local-state",
	Short: "Un-enroll in a course",
	Long: `
About:
When a student is ready to leave a course, they can un-enroll. Un-enrollment can happen any time, whether the student has completed all course modules or not.

This transaction un-enrolls userAccessToken in the course specified by policy.

In this transaction, any earned course credentials are moved to the access token credentials of userAccessToken.

The transaction must be signed by the holder of userAccessToken.

`,
	Run: func(cmd *cobra.Command, args []string) {
		client.GetBurnLocalState(userAccessToken, policy)
	},
}

func init() {
	BurnLocalStateCmd.Flags().StringVar(&userAccessToken, "userAccessToken", "", "Cardano Asset ID of student access token. The wallet holding this asset must sign the generated transaction.")
	BurnLocalStateCmd.Flags().StringVar(&policy, "policy", "", "Course NFT policy id")

	// Required flags
	BurnLocalStateCmd.MarkFlagRequired("userAccessToken")
	BurnLocalStateCmd.MarkFlagRequired("policy")
}
