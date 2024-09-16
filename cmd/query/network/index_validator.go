package network

import (
	"fmt"
	"os"

	index_validator "github.com/Andamio-Platform/andamio-cli/cmd/query/network/index-validator"
	"github.com/spf13/cobra"
)

var IndexValidatorCmd = &cobra.Command{
	Use:   "index-validator",
	Short: "",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {

		// If no arguments are passed, show help
		if len(args) == 0 {
			cmd.Help()
			return
		}

		// If an invalid subcommand is passed, show an error message
		fmt.Printf("Error: '%s' is not a valid subcommand for 'index-validator'\n", args[0])
		fmt.Println("Run './andamio-cli query network index-validator --help' for available subcommands.")
		os.Exit(1) // Exit with a non-zero status to indicate an error
	},
}

func init() {
	IndexValidatorCmd.AddCommand(index_validator.AllIndexValidatorUtxosCmd)
	IndexValidatorCmd.AddCommand(index_validator.InputUtxoCmd)
}
