package global_state

import (
	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/spf13/cobra"
)

var AllGlobalStateUtxosCmd = &cobra.Command{
	Use:   "all-global-state-utxos",
	Short: "Check alias availability",
	Long:  `Check whether a given alias is available.`,
	Run: func(cmd *cobra.Command, args []string) {
		client.GetAllGlobalStateUtxos()
	},
}
