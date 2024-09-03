package instance_validator

import (
	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/spf13/cobra"
)

var AllInstanceValidatorUtxosCmd = &cobra.Command{
	Use:   "all-instance-validator-utxos",
	Short: "",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		client.GetAllInstanceValidatorUtxos()
	},
}
