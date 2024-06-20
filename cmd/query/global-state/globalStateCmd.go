package globalState

import (
	"fmt"

	"github.com/spf13/cobra"
)

var GlobalStateCmd = &cobra.Command{
	Use:   "global-state",
	Short: "Example for Cardano Go PBL",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Querying Andamio Global State...")
		globalState()
	},
}

func init() {
	// AdderPublisherCmd.AddCommand(walletToWallet.WalletToWalletCmd)
}
