package adderPublisher

import (
	"fmt"

	"github.com/spf13/cobra"
)

var ExampleSyncCmd = &cobra.Command{
	Use:   "example-sync",
	Short: "Example for Cardano Go PBL",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Starting sync...")
		SyncExample()
	},
}

func init() {
	// AdderPublisherCmd.AddCommand(walletToWallet.WalletToWalletCmd)
}
