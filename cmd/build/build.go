/*
Copyright © 2024 NAME HERE <EMAIL ADDRESS>
*/
package build

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// buildCmd represents the build command
var BuildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build Andamio transactions",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		// If no arguments are passed, show help
		if len(args) == 0 {
			cmd.Help()
			return
		}

		// If an invalid subcommand is passed, show an error message
		fmt.Printf("Error: '%s' is not a valid subcommand for 'build'\n", args[0])
		fmt.Println("Run './andamio-cli build --help' for available subcommands.")
		os.Exit(1) // Exit with a non-zero status to indicate an error
	},
}

func addBuildSubcommandIslands() {
	// WriteCmd.AddCommand(playground.PlaygroundCmd)
	BuildCmd.AddCommand(TransactionCmd)
}

func init() {
	addBuildSubcommandIslands()

	// Add subcommands under `build`

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// buildCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// buildCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
