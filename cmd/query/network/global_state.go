package network

import (
	"fmt"
	"os"

	global_state "github.com/Andamio-Platform/andamio-cli/cmd/query/network/global-state"
	"github.com/spf13/cobra"
)

var GlobalStateCmd = &cobra.Command{
	Use:   "global-state",
	Short: "change this",
	Long:  `change this.`,
	Run: func(cmd *cobra.Command, args []string) {

		// If no arguments are passed, show help
		if len(args) == 0 {
			cmd.Help()
			return
		}

		// If an invalid subcommand is passed, show an error message
		fmt.Printf("Error: '%s' is not a valid subcommand for 'network'\n", args[0])
		fmt.Println("Run './andamio-cli query network --help' for available subcommands.")
		os.Exit(1) // Exit with a non-zero status to indicate an error
	},
}

func init() {
	GlobalStateCmd.AddCommand(global_state.AllGlobalStateUtxosCmd)
	GlobalStateCmd.AddCommand(global_state.GlobalStateUtxoCmd)
	GlobalStateCmd.AddCommand(global_state.DecodedGlobalStateDatumCmd)
}
