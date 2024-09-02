package course_creator_actions

import (
	"fmt"

	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/spf13/cobra"
)

var moduleInfos string

var MintModuleTokensCmd = &cobra.Command{
	Use:   "mint-module-tokens",
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
		if moduleInfos == "" {
			fmt.Println("Please provide an moduleInfos using --moduleInfos flag")
			return
		}
		client.GetMintModuleTokens(userAccessToken, policy, moduleInfos)
	},
}

func init() {
	MintModuleTokensCmd.Flags().StringVar(&userAccessToken, "userAccessToken", "", "userAccessToken to check availability for")
	MintModuleTokensCmd.Flags().StringVar(&policy, "policy", "", "policy to check availability for")
	MintModuleTokensCmd.Flags().StringVar(&moduleInfos, "moduleInfos", "", "moduleInfos to check availability for")
}
