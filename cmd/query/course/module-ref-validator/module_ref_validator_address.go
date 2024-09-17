package module_ref_validator

import (
	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/spf13/cobra"
)

var ModuleRefValidatorAddressCmd = &cobra.Command{
	Use:   "module-ref-validator-address",
	Short: "",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		client.GetModuleRefValidatorAddress(policy)
	},
}

func init() {
	ModuleRefValidatorAddressCmd.Flags().StringVar(&policy, "policy", "", "")

	ModuleRefValidatorAddressCmd.MarkFlagRequired("policy")
}
