package module_ref_validator

import (
	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/spf13/cobra"
)

var policy string

var DecodedModuleRefDatumsCmd = &cobra.Command{
	Use:   "decoded-module-ref-datums",
	Short: "",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		client.GetDecodedModuleRefDatums(policy)
	},
}

func init() {
	DecodedModuleRefDatumsCmd.Flags().StringVar(&policy, "policy", "", "")

	DecodedModuleRefDatumsCmd.MarkFlagRequired("policy")
}
