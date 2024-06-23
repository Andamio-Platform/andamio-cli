package transaction

import (
	"fmt"

	"github.com/Andamio-Platform/andamio-cli/cmd/transaction/txBuilders/walletToWallet"
	"github.com/spf13/cobra"
)

var TransactionCmd = &cobra.Command{
	Use:   "transaction",
	Short: "Transaction building commands",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(`
andamio-cli transaction

Usage: andamio-cli transaction
( walletToWallet )`)
	},
}

func init() {
	TransactionCmd.AddCommand(walletToWallet.WalletToWalletCmd)
}
