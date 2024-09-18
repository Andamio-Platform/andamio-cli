package global_state

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var GlobalStateCmd = &cobra.Command{
	Use:   "global-state",
	Short: "View Andamio Network data",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		// If no arguments are passed, show help
		if len(args) == 0 {
			cmd.Help()
			return
		}

		// If an invalid subcommand is passed, show an error message
		fmt.Printf("Error: '%s' is not a valid subcommand for 'global-state'\n", args[0])
		fmt.Println("Run './andamio-cli query network global-state --help' for available subcommands.")
		os.Exit(1) // Exit with a non-zero status to indicate an error
	},
}

func init() {
	GlobalStateCmd.AddCommand(AllGlobalStateUtxosCmd)
	GlobalStateCmd.AddCommand(GlobalStateUtxoCmd)
	GlobalStateCmd.AddCommand(DecodedGlobalStateDatumCmd)
}
