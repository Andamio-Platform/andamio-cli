/*
Copyright © 2024 NAME HERE <EMAIL ADDRESS>
*/
package nftMetadata

import (
	"fmt"

	"github.com/spf13/cobra"
)

// nftMetadataCmd represents the writeMetadata command
var NftMetadataCmd = &cobra.Command{
	Use:   "nft-metadata",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("nft-metadata called")
		writeNftFile()
	},
}

func init() {
	// rootCmd.AddCommand(writeMetadataCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// writeMetadataCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// writeMetadataCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
