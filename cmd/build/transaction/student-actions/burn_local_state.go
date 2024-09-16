package student_actions

import (
	"fmt"

	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/spf13/cobra"
)

var userAccessToken string
var policy string

var BurnLocalStateCmd = &cobra.Command{
	Use:   "burn-local-state",
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
		client.GetBurnLocalState(userAccessToken, policy)
	},
}

func init() {
	BurnLocalStateCmd.Flags().StringVar(&userAccessToken, "userAccessToken", "", "")
	BurnLocalStateCmd.Flags().StringVar(&policy, "policy", "", "")
}
