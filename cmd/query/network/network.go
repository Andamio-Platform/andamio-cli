package network

import (
	"fmt"
	"os"

	global_state "github.com/Andamio-Platform/andamio-cli/cmd/query/network/global-state"
	index_validator "github.com/Andamio-Platform/andamio-cli/cmd/query/network/index-validator"
	instance_validator "github.com/Andamio-Platform/andamio-cli/cmd/query/network/instance-validator"
	"github.com/spf13/cobra"
)

var NetworkCmd = &cobra.Command{
	Use:   "network",
	Short: "View Andamio network data",
	Long:  ``,
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
	NetworkCmd.AddCommand(AliasAvailabilityCmd)
	NetworkCmd.AddCommand(global_state.GlobalStateCmd)
	NetworkCmd.AddCommand(index_validator.IndexValidatorCmd)
	NetworkCmd.AddCommand(instance_validator.InstanceValidatorCmd)
}
