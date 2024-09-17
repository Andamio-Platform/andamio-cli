package global_state

import (
	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/spf13/cobra"
)

var GlobalStateUtxoCmd = &cobra.Command{
	Use:   "global-state-utxo",
	Short: "View global state utxo for specified alias",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		client.GetGlobalStateUtxo(alias)
	},
}

func init() {
	GlobalStateUtxoCmd.Flags().StringVar(&alias, "alias", "", "")

	GlobalStateUtxoCmd.MarkFlagRequired("alias")
}
