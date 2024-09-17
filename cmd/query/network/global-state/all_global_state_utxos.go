package global_state

import (
	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/spf13/cobra"
)

var AllGlobalStateUtxosCmd = &cobra.Command{
	Use:   "all-global-state-utxos",
	Short: "List UTxOs for all access tokens on Andamio Network",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		client.GetAllGlobalStateUtxos()
	},
}
