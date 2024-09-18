package student_actions

import (
	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/spf13/cobra"
)

var MintLocalStateCmd = &cobra.Command{
	Use:   "mint-local-state",
	Short: "Enroll in a course on Andamio network",
	Long: `
About:
The holder of an access token can enroll in courses on the Andamio Network.

This transaction enrolls userAccessToken in the course specified by policy.

The transaction must be signed by the holder of userAccessToken.

`,
	Run: func(cmd *cobra.Command, args []string) {
		client.GetMintLocalState(userAccessToken, policy)
	},
}

func init() {
	MintLocalStateCmd.Flags().StringVar(&userAccessToken, "userAccessToken", "", "Cardano Asset ID of student access token. The wallet holding this asset must sign the generated transaction.")
	MintLocalStateCmd.Flags().StringVar(&policy, "policy", "", "Course NFT policy id")

	// Required flags
	MintLocalStateCmd.MarkFlagRequired("userAccessToken")
	MintLocalStateCmd.MarkFlagRequired("policy")
}
