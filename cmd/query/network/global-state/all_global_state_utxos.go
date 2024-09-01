package global_state

import (
	"fmt"

	"github.com/spf13/cobra"
)

var AllGlobalStateUtxosCmd = &cobra.Command{
	Use:   "all-global-state-utxos",
	Short: "Check alias availability",
	Long:  `Check whether a given alias is available.`,
	Run: func(cmd *cobra.Command, args []string) {
		if alias == "" {
			fmt.Println("Please provide an alias using --alias flag")
			return
		}
		fmt.Printf("Checking availability for alias: %s\n", alias)
		// Your alias availability logic here
	},
}
