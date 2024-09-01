package module_ref_validator

import (
	"fmt"

	"github.com/spf13/cobra"
)

var ModuleRefValidatorUtxoCmd = &cobra.Command{
	Use:   "module-ref-validator-utxo",
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

func init() {
	ModuleRefValidatorUtxoCmd.Flags().StringVar(&alias, "alias", "", "Alias to check availability for")
}
