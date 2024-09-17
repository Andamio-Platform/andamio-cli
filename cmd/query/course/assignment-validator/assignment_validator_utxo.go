package assignment_validator

import (
	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/spf13/cobra"
)

var AssignmentValidatorUtxoCmd = &cobra.Command{
	Use:   "assignment-validator-utxo",
	Short: "View commitment UTxO currently locked at assignment validator address for specified alias",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		client.GetAssignmentValidatorUtxo(policy, alias)
	},
}

func init() {
	AssignmentValidatorUtxoCmd.Flags().StringVar(&alias, "alias", "", "")
	AssignmentValidatorUtxoCmd.Flags().StringVar(&policy, "policy", "", "")

	AssignmentValidatorAddressCmd.MarkFlagRequired("alias")
	AssignmentValidatorAddressCmd.MarkFlagRequired("policy")
}
