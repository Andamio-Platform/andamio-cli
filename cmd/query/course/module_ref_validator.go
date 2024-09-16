package course

import (
	"fmt"
	"os"

	module_ref_validator "github.com/Andamio-Platform/andamio-cli/cmd/query/course/module-ref-validator"
	"github.com/spf13/cobra"
)

var ModuleRefValidatorCmd = &cobra.Command{
	Use:   "module-ref-validator",
	Short: "",
	Long:  `.`,
	Run: func(cmd *cobra.Command, args []string) {

		// If no arguments are passed, show help
		if len(args) == 0 {
			cmd.Help()
			return
		}

		// If an invalid subcommand is passed, show an error message
		fmt.Printf("Error: '%s' is not a valid subcommand for 'module-ref-validator'\n", args[0])
		fmt.Println("Run './andamio-cli query course module-ref-validator --help' for available subcommands.")
		os.Exit(1) // Exit with a non-zero status to indicate an error
	},
}

func init() {
	ModuleRefValidatorCmd.AddCommand(module_ref_validator.DecodedModuleRefDatumsCmd)
	ModuleRefValidatorCmd.AddCommand(module_ref_validator.ModuleRefValidatorAddressCmd)
	ModuleRefValidatorCmd.AddCommand(module_ref_validator.ModuleRefValidatorUtxoCmd)
	ModuleRefValidatorCmd.AddCommand(module_ref_validator.ModuleRefValidatorUtxosCmd)
}
