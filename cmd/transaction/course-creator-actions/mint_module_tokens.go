package course_creator_actions

import (
	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/spf13/cobra"
)

var moduleInfos string

var MintModuleTokensCmd = &cobra.Command{
	Use:   "mint-module-tokens",
	Short: "Publish course credential criteria on-chain",
	Long: `
About:
Before a student can commit to an assignment, the course creator must publish credential criteria on-chain.

This transaction mints course module tokens specifying Student Learning Targets (SLTs) and an assignment for each course module.

The transaction must be signed by the holder of userAccessToken.

Example:
  andamio-cli transaction course-creator mint-module-tokens \ 
    --userAccessToken ASSET_ID \
    --policy POLICY_ID \
    --moduleInfos STRING 

  `,
	Run: func(cmd *cobra.Command, args []string) {
		client.GetMintModuleTokens(userAccessToken, policy, moduleInfos)
	},
}

func init() {
	MintModuleTokensCmd.Flags().StringVar(&userAccessToken, "userAccessToken", "", "Cardano Asset ID of teacher access token. The wallet holding this asset must sign the generated transaction.")
	MintModuleTokensCmd.Flags().StringVar(&policy, "policy", "", "Course NFT policy id")
	MintModuleTokensCmd.Flags().StringVar(&moduleInfos, "moduleInfos", "", "List of course module information. Use andamio-cli write module-info to generate valid module-info")

	// Required Flags
	MintModuleTokensCmd.MarkFlagRequired("userAccessToken")
	MintModuleTokensCmd.MarkFlagRequired("policy")
	MintModuleTokensCmd.MarkFlagRequired("moduleInfos")
}
