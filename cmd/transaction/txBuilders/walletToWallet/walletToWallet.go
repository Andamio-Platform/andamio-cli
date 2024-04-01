package walletToWallet

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	originAddress string
	// vKeyFilePath       string
	// sKeyFilePath       string
	destinationAddress string
	amount             float64
	submitOpt          int64
)
var WalletToWalletCmd = &cobra.Command{
	Use:   "wallet-to-wallet",
	Short: "Wallet to wallet tx building command",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {

		txHash, err := WalletToWalletTxBuilder(originAddress, destinationAddress, amount, submitOpt)
		if err != nil {
			panic(err)
		}

		if txHash != "" {
			fmt.Println("TxHash:", txHash)
		}

	},
}

func init() {

	WalletToWalletCmd.PersistentFlags().StringVarP(&originAddress, "origin-address", "o", "", "origin address")
	// WalletToWalletCmd.PersistentFlags().StringVarP(&vKeyFilePath, "vkey-file-path", "v", "", "vkey file path")
	// WalletToWalletCmd.PersistentFlags().StringVarP(&sKeyFilePath, "skey-file-path", "s", "", "skey file path")
	WalletToWalletCmd.PersistentFlags().StringVarP(&destinationAddress, "destination-address", "d", "", "destination address")
	WalletToWalletCmd.PersistentFlags().Float64VarP(&amount, "amount", "a", 0, "amount")
	WalletToWalletCmd.PersistentFlags().Int64VarP(&submitOpt, "submit-opt", "p", 0, "submit option")

}
