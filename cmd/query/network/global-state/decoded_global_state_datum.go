package global_state

import (
	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/spf13/cobra"
)

var alias string

var DecodedGlobalStateDatumCmd = &cobra.Command{
	Use:   "decoded-global-state-datum",
	Short: "View access token datum for specified alias",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		client.GetDecodedGlobalStateDatum(alias)
	},
}

func init() {
	DecodedGlobalStateDatumCmd.Flags().StringVar(&alias, "alias", "", "")

	DecodedGlobalStateDatumCmd.MarkFlagRequired("alias")
}
