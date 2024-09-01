package instance_validator

import (
	"fmt"

	"github.com/spf13/cobra"
)

var ModuleTokenPolicyRefUtxoCmd = &cobra.Command{
	Use:   "module-token-policy-ref-utxo",
	Short: "Check alias availability",
	Long:  `Check whether a given alias is available.`,
	Run: func(cmd *cobra.Command, args []string) {
		if alias == "" {
			fmt.Println("Please provide an alias using --alias flag")
			return
		}
		fmt.Printf("Checking availability for alias: %s\n", alias)
		// Your alias availability logic here
	},
}

func init() {
	ModuleTokenPolicyRefUtxoCmd.Flags().StringVar(&alias, "alias", "", "Alias to check availability for")
}
