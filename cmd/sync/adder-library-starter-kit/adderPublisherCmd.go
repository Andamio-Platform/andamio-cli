package adderPublisher

import (
	"fmt"

	"github.com/spf13/cobra"
)

var ExampleSyncCmd = &cobra.Command{
	Use:   "example-sync",
	Short: "Example for Cardano Go PBL",
	Long: `

This command is a wrapper around the Adder Library Starter Kit:
https://github.com/blinklabs-io/adder-library-starter-kit
	
	`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Starting sync...")
		SyncExample()
	},
}

func init() {
	// AdderPublisherCmd.AddCommand(walletToWallet.WalletToWalletCmd)
}
