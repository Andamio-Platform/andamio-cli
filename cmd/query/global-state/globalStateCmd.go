package globalState

import (
	"fmt"

	"github.com/spf13/cobra"
)

var tokenName string

var GlobalStateCmd = &cobra.Command{
	Use:   "global-state",
	Short: "Example for Cardano Go PBL",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Querying Andamio Global State...")
		globalState(tokenName)
	},
}

func init() {
	// AdderPublisherCmd.AddCommand(walletToWallet.WalletToWalletCmd)
	GlobalStateCmd.Flags().StringVarP(&tokenName, "tokenName", "n", "", "Optionally specify a token name")
}
