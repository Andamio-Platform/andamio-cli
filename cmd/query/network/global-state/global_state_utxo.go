package global_state

import (
	"fmt"

	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/spf13/cobra"
)

var GlobalStateUtxoCmd = &cobra.Command{
	Use:   "global-state-utxo",
	Short: "",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		if alias == "" {
			fmt.Println("Please provide an alias using --alias flag")
			return
		}
		client.GetGlobalStateUtxo(alias)
	},
}

func init() {
	GlobalStateUtxoCmd.Flags().StringVar(&alias, "alias", "", "")
}
