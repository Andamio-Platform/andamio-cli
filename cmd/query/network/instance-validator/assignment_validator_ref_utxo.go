package instance_validator

import (
	"fmt"

	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/spf13/cobra"
)

var policy string

var AssignmentValidatorRefUtxoCmd = &cobra.Command{
	Use:   "assignment-validator-ref-utxo",
	Short: "",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		if policy == "" {
			fmt.Println("Please provide an policy using --policy flag")
			return
		}

		client.GetAssignmentValidatorRefUtxo(policy)
	},
}

func init() {
	AssignmentValidatorRefUtxoCmd.Flags().StringVar(&policy, "policy", "", "")
}
