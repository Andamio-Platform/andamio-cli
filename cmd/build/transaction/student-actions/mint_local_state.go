package student_actions

import (
	"fmt"

	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/spf13/cobra"
)

var MintLocalStateCmd = &cobra.Command{
	Use:   "mint-local-state",
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
		client.GetMintLocalState(userAccessToken, policy)
	},
}

func init() {
	MintLocalStateCmd.Flags().StringVar(&userAccessToken, "userAccessToken", "", "userAccessToken to check availability for")
	MintLocalStateCmd.Flags().StringVar(&policy, "policy", "", "policy to check availability for")
}
