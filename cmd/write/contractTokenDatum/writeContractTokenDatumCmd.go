package writeContractTokenDatum

import (
	"fmt"

	"github.com/spf13/cobra"
)

var inputFileName string

var ContractTokenDatumCmd = &cobra.Command{
	Use:   "global-state",
	Short: "Example for Cardano Go PBL",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Querying Andamio Global State...")
		writeContractTokenDatum()
	},
}

func init() {
	// AdderPublisherCmd.AddCommand(walletToWallet.WalletToWalletCmd)
	ContractTokenDatumCmd.Flags().StringVarP(&inputFileName, "inputFileName", "i", "", "Name of input file")
}
