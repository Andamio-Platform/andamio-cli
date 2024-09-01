package module_ref_validator

import (
	"fmt"

	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/spf13/cobra"
)

var token_name string

var ModuleRefValidatorUtxoCmd = &cobra.Command{
	Use:   "module-ref-validator-utxo",
	Short: "Check policy availability",
	Long:  `Check whether a given policy is available.`,
	Run: func(cmd *cobra.Command, args []string) {
		if policy == "" {
			fmt.Println("Please provide an policy using --policy flag")
			return
		}
		if token_name == "" {
			fmt.Println("Please provide an policy using --token-name flag")
			return
		}
		fmt.Printf("Checking availability for policy: %s\n", policy)
		// Your policy availability logic here
		client.GetModuleRefValidatorUtxo(policy, token_name)
	},
}

func init() {
	ModuleRefValidatorUtxoCmd.Flags().StringVar(&policy, "policy", "", "policy to check availability for")
	ModuleRefValidatorUtxoCmd.Flags().StringVar(&token_name, "token-name", "", "token_name to check availability for")
}
