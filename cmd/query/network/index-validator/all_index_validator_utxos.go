package index_validator

import (
	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/spf13/cobra"
)

var alias string

var AllIndexValidatorUtxosCmd = &cobra.Command{
	Use:   "all-index-validator-utxos",
	Short: "",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		client.GetAllIndexValidatorUtxos()
	},
}
