package assignment_validator

import (
	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/spf13/cobra"
)

var DecodedAssignmentValidatorUtxoDatum = &cobra.Command{
	Use:   "decoded-assignment-validator-utxo-datum",
	Short: "",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		client.GetDecodedAssignmentValidatorUtxoDatum(policy, alias)
	},
}

func init() {
	DecodedAssignmentValidatorUtxoDatum.Flags().StringVar(&alias, "alias", "", "")
	DecodedAssignmentValidatorUtxoDatum.Flags().StringVar(&policy, "policy", "", "")

	DecodedAssignmentValidatorUtxoDatum.MarkFlagRequired("alias")
	DecodedAssignmentValidatorUtxoDatum.MarkFlagRequired("policy")
}
