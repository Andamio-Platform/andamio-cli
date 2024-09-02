package transaction

import (
	"fmt"

	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/spf13/cobra"
)

var userAddress string
var alias string
var userInfo string

var MintAccessTokenCmd = &cobra.Command{
	Use:   "mint-access-token",
	Short: "Build transaction",
	Long:  `Build transactions.`,
	Run: func(cmd *cobra.Command, args []string) {
		if userAddress == "" {
			fmt.Println("Please provide an userAddress using --userAddress flag")
			return
		}
		if alias == "" {
			fmt.Println("Please provide an alias using --alias flag")
			return
		}
		if userInfo == "" {
			fmt.Println("Please provide an userInfo using --userInfo flag")
			return
		}
		client.GetMintAccessToken(userAddress, alias, userInfo)
	},
}

func init() {
	MintAccessTokenCmd.Flags().StringVar(&userAddress, "userAddress", "", "userAddress to check availability for")
	MintAccessTokenCmd.Flags().StringVar(&alias, "alias", "", "alias to check availability for")
	MintAccessTokenCmd.Flags().StringVar(&userInfo, "userInfo", "", "userInfo to check availability for")
}
