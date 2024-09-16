package assignment_validator

import (
	"fmt"

	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/spf13/cobra"
)

var AssignmentValidatorUtxoCmd = &cobra.Command{
	Use:   "assignment-validator-utxo",
	Short: "",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		if policy == "" {
			fmt.Println("Please provide an policy using --policy flag")
			return
		}
		if alias == "" {
			fmt.Println("Please provide an alias using --alias flag")
			return
		}

		client.GetAssignmentValidatorUtxo(policy, alias)
	},
}

func init() {
	AssignmentValidatorUtxoCmd.Flags().StringVar(&alias, "alias", "", "")
	AssignmentValidatorUtxoCmd.Flags().StringVar(&policy, "policy", "", "")
}
