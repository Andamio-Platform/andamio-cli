package instance_validator

import (
	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/spf13/cobra"
)

var ModuleTokenPolicyRefUtxoCmd = &cobra.Command{
	Use:   "module-token-policy-ref-utxo",
	Short: "View the module token minting reference UTxO for course with specified policy",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		client.GetModuleTokenPolicyRefUtxo(policy)
	},
}

func init() {
	ModuleTokenPolicyRefUtxoCmd.Flags().StringVar(&policy, "policy", "", "")

	ModuleTokenPolicyRefUtxoCmd.MarkFlagRequired("policy")
}
