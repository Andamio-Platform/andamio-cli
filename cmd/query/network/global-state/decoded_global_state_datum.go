package global_state

import (
	"andamio-cli/utils"
	"log"

	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/spf13/cobra"
)

var alias string

var DecodedGlobalStateDatumCmd = &cobra.Command{
	Use:   "view-access-token",
	Short: "View access token datum for specified alias",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		response, err := client.GetDecodedGlobalStateDatum(alias)
		if err != nil {
			log.Fatalf("Falied to get list of access token datums: %v", err)
		}

		utils.SaveOutputToFile(cmd, response)
	},
}

func init() {
	DecodedGlobalStateDatumCmd.Flags().StringVar(&alias, "alias", "", "")

	DecodedGlobalStateDatumCmd.MarkFlagRequired("alias")

	utils.AddOutFileFlag(DecodedGlobalStateDatumCmd)
}
