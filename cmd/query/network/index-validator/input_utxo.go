package index_validator

import (
	"fmt"

	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/spf13/cobra"
)

var InputUtxoCmd = &cobra.Command{
	Use:   "input-utxo",
	Short: "Check alias availability",
	Long:  `Check whether a given alias is available.`,
	Run: func(cmd *cobra.Command, args []string) {
		if alias == "" {
			fmt.Println("Please provide an alias using --alias flag")
			return
		}
		fmt.Printf("Checking availability for alias: %s\n", alias)
		// Your alias availability logic here
		client.GetInputUtxo(alias)
	},
}

func init() {
	InputUtxoCmd.Flags().StringVar(&alias, "alias", "", "Alias to check availability for")
}
