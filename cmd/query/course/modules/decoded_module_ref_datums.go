package module_ref_validator

import (
	"log"

	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/Andamio-Platform/andamio-cli/utils"
	"github.com/spf13/cobra"
)

var policy string

var DecodedModuleRefDatumsCmd = &cobra.Command{
	Use:   "list",
	Short: "View module datum for course with specified policy",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		response, err := client.GetDecodedModuleRefDatums(policy)
		if err != nil {
			log.Fatalf("Falied to get list of module datums: %v", err)
		}

		utils.SaveOutputToFile(cmd, response)
	},
}

func init() {
	DecodedModuleRefDatumsCmd.Flags().StringVar(&policy, "policy", "", "")

	DecodedModuleRefDatumsCmd.MarkFlagRequired("policy")

	utils.AddOutFileFlag(DecodedModuleRefDatumsCmd)
}
