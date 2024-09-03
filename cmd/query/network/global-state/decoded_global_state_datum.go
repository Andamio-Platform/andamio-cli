package global_state

import (
	"fmt"

	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/spf13/cobra"
)

var alias string

var DecodedGlobalStateDatumCmd = &cobra.Command{
	Use:   "decoded-global-state-datum",
	Short: "",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		if alias == "" {
			fmt.Println("Please provide an alias using --alias flag")
			return
		}
		client.GetDecodedGlobalStateDatum(alias)
	},
}

func init() {
	DecodedGlobalStateDatumCmd.Flags().StringVar(&alias, "alias", "", "")
}
