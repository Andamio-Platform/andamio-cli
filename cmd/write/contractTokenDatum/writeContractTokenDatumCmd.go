package writeContractTokenDatum

import (
	"fmt"

	"github.com/spf13/cobra"
)

var inputFileName string

var ContractTokenDatumCmd = &cobra.Command{
	Use:   "contract-token-datum",
	Short: "Example for Cardano Go PBL",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Writing contract token datum from input.json...")
		writeContractTokenDatum(inputFileName)
	},
}

func init() {
	// AdderPublisherCmd.AddCommand(walletToWallet.WalletToWalletCmd)
	ContractTokenDatumCmd.Flags().StringVarP(&inputFileName, "inputFileName", "i", "", "Name of input file")
}
