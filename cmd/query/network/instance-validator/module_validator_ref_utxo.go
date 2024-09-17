package instance_validator

import (
	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/spf13/cobra"
)

var ModuleValidatorRefUtxoCmd = &cobra.Command{
	Use:   "module-validator-ref-utxo",
	Short: "View the module validator reference UTxO for course with specified policy",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		client.GetModuleValidatorRefUtxo(policy)
	},
}

func init() {
	ModuleValidatorRefUtxoCmd.Flags().StringVar(&policy, "policy", "", "")

	ModuleValidatorRefUtxoCmd.MarkFlagRequired("policy")
}
