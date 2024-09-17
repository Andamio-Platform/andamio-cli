package module_ref_validator

import (
	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/spf13/cobra"
)

var token_name string

var ModuleRefValidatorUtxoCmd = &cobra.Command{
	Use:   "module-ref-validator-utxo",
	Short: "View module reference utxo for module with specified token-name in course with specified policy",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		client.GetModuleRefValidatorUtxo(policy, token_name)
	},
}

func init() {
	ModuleRefValidatorUtxoCmd.Flags().StringVar(&policy, "policy", "", "")
	ModuleRefValidatorUtxoCmd.Flags().StringVar(&token_name, "token-name", "", "")

	ModuleRefValidatorUtxoCmd.MarkFlagRequired("policy")
	ModuleRefValidatorUtxoCmd.MarkFlagRequired("token-name")
}
