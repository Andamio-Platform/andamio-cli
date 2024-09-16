package assignment_validator

import (
	"fmt"

	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/spf13/cobra"
)

var AssignmentValidatorUtxosCmd = &cobra.Command{
	Use:   "assignment-validator-utxos",
	Short: "",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		if policy == "" {
			fmt.Println("Please provide an policy using --policy flag")
			return
		}

		client.GetAssignmentValidatorUtxos(policy)
	},
}

func init() {
	AssignmentValidatorUtxosCmd.Flags().StringVar(&policy, "policy", "", "")
}
