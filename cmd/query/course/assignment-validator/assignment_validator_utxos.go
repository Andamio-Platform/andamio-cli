package assignment_validator

import (
	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/spf13/cobra"
)

var AssignmentValidatorUtxosCmd = &cobra.Command{
	Use:   "assignment-validator-utxos",
	Short: "View all commitment UTxOs currently locked at assignment validator address",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		client.GetAssignmentValidatorUtxos(policy)
	},
}

func init() {
	AssignmentValidatorUtxosCmd.Flags().StringVar(&policy, "policy", "", "")

	AssignmentValidatorUtxosCmd.MarkFlagRequired("policy")
}
