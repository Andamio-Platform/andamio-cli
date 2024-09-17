package module_ref_validator

import (
	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/spf13/cobra"
)

var ModuleRefValidatorUtxosCmd = &cobra.Command{
	Use:   "module-ref-validator-utxos",
	Short: "",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		client.GetModuleRefValidatorUtxos(policy)
	},
}

func init() {
	ModuleRefValidatorUtxosCmd.Flags().StringVar(&policy, "policy", "", "")

	ModuleRefValidatorUtxosCmd.MarkFlagRequired("policy")
}
