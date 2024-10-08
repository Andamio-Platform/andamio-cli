package assignment_validator

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var AssignmentValidatorCmd = &cobra.Command{
	Use:   "assignments",
	Short: "View network assignment data",
	Long:  `.`,
	Run: func(cmd *cobra.Command, args []string) {
		// If no arguments are passed, show help
		if len(args) == 0 {
			cmd.Help()
			return
		}

		// If an invalid subcommand is passed, show an error message
		fmt.Printf("Error: '%s' is not a valid subcommand for 'assignment-validator'\n", args[0])
		fmt.Println("Run './andamio-cli query course assignment-validator --help' for available subcommands.")
		os.Exit(1) // Exit with a non-zero status to indicate an error
	},
}

func init() {
	AssignmentValidatorCmd.AddCommand(AssignmentValidatorAddressCmd)
	AssignmentValidatorCmd.AddCommand(AssignmentValidatorUtxoCmd)
	AssignmentValidatorCmd.AddCommand(AssignmentValidatorUtxosCmd)
	AssignmentValidatorCmd.AddCommand(DecodedAssignmentDatums)
	AssignmentValidatorCmd.AddCommand(DecodedAssignmentValidatorUtxoDatum)
}
