/*
Copyright © 2024 NAME HERE <EMAIL ADDRESS>
*/
package write

import (
	writeContractTokenDatum "github.com/Andamio-Platform/andamio-cli/cmd/write/contractTokenDatum"
	module_info "github.com/Andamio-Platform/andamio-cli/cmd/write/moduleInfo"
	"github.com/Andamio-Platform/andamio-cli/cmd/write/nftMetadata"
	"github.com/spf13/cobra"
)

// writeCmd represents the write command
var WriteCmd = &cobra.Command{
	Use:   "write",
	Short: "Write data functions",
	Long: `

Write data functions

	`,
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 0 {
			cmd.Help()
			return
		}
	},
}

func addWriteSubcommandIslands() {
	// WriteCmd.AddCommand(playground.PlaygroundCmd)
	WriteCmd.AddCommand(nftMetadata.NftMetadataCmd)
	WriteCmd.AddCommand(writeContractTokenDatum.ContractTokenDatumCmd)
	WriteCmd.AddCommand(module_info.ModuleInfoCmd)
}

func init() {
	addWriteSubcommandIslands()

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// writeCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// writeCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
