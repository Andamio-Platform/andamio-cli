package instance_validator

import (
	"fmt"

	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/spf13/cobra"
)

var LocalStateValidatorRefUtxoCmd = &cobra.Command{
	Use:   "local-state-validator-ref-utxo",
	Short: "Check policy availability",
	Long:  `Check whether a given policy is available.`,
	Run: func(cmd *cobra.Command, args []string) {
		if policy == "" {
			fmt.Println("Please provide an policy using --policy flag")
			return
		}
		fmt.Printf("Checking availability for policy: %s\n", policy)
		// Your policy availability logic here
		client.GetLocalStateValidatorRefUtxo(policy)
	},
}

func init() {
	LocalStateValidatorRefUtxoCmd.Flags().StringVar(&policy, "policy", "", "policy to check availability for")
}
