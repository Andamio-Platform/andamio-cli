package writeContractTokenDatum

import (
	"fmt"

	"github.com/spf13/cobra"
)

var inputFileName string

var ContractTokenDatumCmd = &cobra.Command{
	Use:   "contract-token-datum",
	Short: "Andamio Course Contract Token Datum",
	Long: `
Query the list of courses currently registered on the Andamio Network
	
	`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("Writing contract token datum from %s...\n", inputFileName)
		writeContractTokenDatum(inputFileName)
	},
}

func init() {
	// AdderPublisherCmd.AddCommand(walletToWallet.WalletToWalletCmd)
	ContractTokenDatumCmd.Flags().StringVarP(&inputFileName, "inputFileName", "i", "", "Name of input file")
}
