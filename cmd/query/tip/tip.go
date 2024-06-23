/*
Copyright © 2024 NAME HERE <EMAIL ADDRESS>
*/
package tip

import (
	"fmt"

	"github.com/spf13/cobra"
)

// tipCmd represents the tip command
var TipCmd = &cobra.Command{
	Use:   "tip",
	Short: "Simple query example for Cardano Go PBL course",
	Long: `
Query tip of Cardano Preview network using Demeter example.

	`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("tip called")
		chainTip()
	},
}

func init() {
	// rootCmd.AddCommand(tipCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// tipCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// tipCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
