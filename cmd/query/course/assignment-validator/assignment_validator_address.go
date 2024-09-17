package assignment_validator

import (
	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/spf13/cobra"
)

var (
	alias  string
	policy string
)

var AssignmentValidatorAddressCmd = &cobra.Command{
	Use:   "assignment-validator-address",
	Short: "",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		client.GetAssignmentValidatorAddresses(policy)
	},
}

func init() {
	AssignmentValidatorAddressCmd.Flags().StringVar(&policy, "policy", "", "")
	AssignmentValidatorAddressCmd.MarkFlagRequired("policy")
}
