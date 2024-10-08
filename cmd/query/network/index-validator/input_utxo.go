package index_validator

import (
	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/spf13/cobra"
)

var InputUtxoCmd = &cobra.Command{
	Use:   "input-utxo",
	Short: "Find input utxo for minting a new access token with specified alias",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		client.GetInputUtxo(alias)
	},
}

func init() {
	InputUtxoCmd.Flags().StringVar(&alias, "alias", "", "")

	InputUtxoCmd.MarkFlagRequired("alias")
}
