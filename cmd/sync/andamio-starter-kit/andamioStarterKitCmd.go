package andamioStarterKit

import (
	"fmt"

	// "andamio-indexer/src/indexer"

	"github.com/spf13/cobra"
)

var AndamioStarterKitCmd = &cobra.Command{
	Use:   "andamio-starter-kit",
	Short: "Extend the Adder Starter Kit to build your own Andamio Indexer",
	Long: `In Cardano Go PBL, you can learn how to build and run your own custom Andamio Indexer.

View the example: https://www.andamio.io/course/gpbl2024
	`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Starting Andamio network sync...")
		// indexer.RunIndexer()
	},
}

func init() {
	// AdderPublisherCmd.AddCommand(walletToWallet.WalletToWalletCmd)
}
