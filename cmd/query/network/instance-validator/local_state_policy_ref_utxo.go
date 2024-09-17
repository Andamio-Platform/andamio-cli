package instance_validator

import (
	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/spf13/cobra"
)

var LocalStatePolicyRefUtxoCmd = &cobra.Command{
	Use:   "local-state-policy-ref-utxo",
	Short: "",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		client.GetLocalStatePolicyRefUtxo(policy)
	},
}

func init() {
	LocalStatePolicyRefUtxoCmd.Flags().StringVar(&policy, "policy", "", "")

	LocalStatePolicyRefUtxoCmd.MarkFlagRequired("policy")
}
