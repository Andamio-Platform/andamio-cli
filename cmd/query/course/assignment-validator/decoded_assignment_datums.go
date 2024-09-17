package assignment_validator

import (
	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/spf13/cobra"
)

var DecodedAssignmentDatums = &cobra.Command{
	Use:   "decoded-assignment-datums",
	Short: "",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		client.GetDecodedAssignmentDatums(policy)
	},
}

func init() {
	DecodedAssignmentDatums.Flags().StringVar(&policy, "policy", "", "")

	DecodedAssignmentDatums.MarkFlagRequired("policy")
}
