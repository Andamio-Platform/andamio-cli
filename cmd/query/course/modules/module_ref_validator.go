package module_ref_validator

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var ModuleRefValidatorCmd = &cobra.Command{
	Use:   "modules",
	Short: "View course module credential details",
	Long:  ``,
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
	ModuleRefValidatorCmd.AddCommand(DecodedModuleRefDatumsCmd)
	ModuleRefValidatorCmd.AddCommand(ModuleRefValidatorAddressCmd)
	ModuleRefValidatorCmd.AddCommand(ModuleRefValidatorUtxoCmd)
	ModuleRefValidatorCmd.AddCommand(ModuleRefValidatorUtxosCmd)
}
