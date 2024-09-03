package module_ref_validator

import (
	"fmt"

	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/spf13/cobra"
)

var ModuleRefValidatorUtxosCmd = &cobra.Command{
	Use:   "module-ref-validator-utxos",
	Short: "",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		if policy == "" {
			fmt.Println("Please provide an policy using --policy flag")
			return
		}

		client.GetModuleRefValidatorUtxos(policy)
	},
}

func init() {
	ModuleRefValidatorUtxosCmd.Flags().StringVar(&policy, "policy", "", "")
}
