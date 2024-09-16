package index_validator

import (
	"fmt"

	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/spf13/cobra"
)

var InputUtxoCmd = &cobra.Command{
	Use:   "input-utxo",
	Short: "",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		if alias == "" {
			fmt.Println("Please provide an alias using --alias flag")
			return
		}
		client.GetInputUtxo(alias)
	},
}

func init() {
	InputUtxoCmd.Flags().StringVar(&alias, "alias", "", "")
}
