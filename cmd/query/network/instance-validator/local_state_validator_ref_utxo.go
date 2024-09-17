package instance_validator

import (
	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/spf13/cobra"
)

var LocalStateValidatorRefUtxoCmd = &cobra.Command{
	Use:   "local-state-validator-ref-utxo",
	Short: "View the local state validator reference UTxO for course with specified policy",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		client.GetLocalStateValidatorRefUtxo(policy)
	},
}

func init() {
	LocalStateValidatorRefUtxoCmd.Flags().StringVar(&policy, "policy", "", "")

	LocalStateValidatorRefUtxoCmd.MarkFlagRequired("policy")
}
