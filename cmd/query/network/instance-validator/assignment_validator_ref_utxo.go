package instance_validator

import (
	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/spf13/cobra"
)

var policy string

var AssignmentValidatorRefUtxoCmd = &cobra.Command{
	Use:   "assignment-validator-ref-utxo",
	Short: "",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		client.GetAssignmentValidatorRefUtxo(policy)
	},
}

func init() {
	AssignmentValidatorRefUtxoCmd.Flags().StringVar(&policy, "policy", "", "")

	AssignmentValidatorRefUtxoCmd.MarkFlagRequired("policy")
}
