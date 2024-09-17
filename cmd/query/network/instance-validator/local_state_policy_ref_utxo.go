package instance_validator

import (
	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/spf13/cobra"
)

var LocalStatePolicyRefUtxoCmd = &cobra.Command{
	Use:   "local-state-policy-ref-utxo",
	Short: "View the local state policy reference UTxO for course with specified policy",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		client.GetLocalStatePolicyRefUtxo(policy)
	},
}

func init() {
	LocalStatePolicyRefUtxoCmd.Flags().StringVar(&policy, "policy", "", "")

	LocalStatePolicyRefUtxoCmd.MarkFlagRequired("policy")
}
