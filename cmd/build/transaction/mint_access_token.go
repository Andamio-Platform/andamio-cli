package transaction

import (
	"github.com/spf13/cobra"
)

var MintAccessTokenCmd = &cobra.Command{
	Use:   "mint-access-token",
	Short: "Build transaction",
	Long:  `Build transactions.`,
	Run: func(cmd *cobra.Command, args []string) {

		// Your access tokens logic here
	},
}
