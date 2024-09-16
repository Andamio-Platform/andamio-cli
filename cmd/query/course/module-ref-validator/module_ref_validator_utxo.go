package module_ref_validator

import (
	"fmt"

	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/spf13/cobra"
)

var token_name string

var ModuleRefValidatorUtxoCmd = &cobra.Command{
	Use:   "module-ref-validator-utxo",
	Short: "",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		if policy == "" {
			fmt.Println("Please provide an policy using --policy flag")
			return
		}
		if token_name == "" {
			fmt.Println("Please provide an policy using --token-name flag")
			return
		}

		client.GetModuleRefValidatorUtxo(policy, token_name)
	},
}

func init() {
	ModuleRefValidatorUtxoCmd.Flags().StringVar(&policy, "policy", "", "")
	ModuleRefValidatorUtxoCmd.Flags().StringVar(&token_name, "token-name", "", "")
}
