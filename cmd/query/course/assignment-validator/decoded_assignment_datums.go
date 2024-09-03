package assignment_validator

import (
	"fmt"

	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/spf13/cobra"
)

var DecodedAssignmentDatums = &cobra.Command{
	Use:   "decoded-assignment-datums",
	Short: "",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		if policy == "" {
			fmt.Println("Please provide an policy using --policy flag")
			return
		}

		client.GetDecodedAssignmentDatums(policy)
	},
}

func init() {
	DecodedAssignmentDatums.Flags().StringVar(&policy, "policy", "", "")
}
