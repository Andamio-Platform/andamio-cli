/*
Copyright © 2024 Andamio dev@andamio.io
*/
package cmd

import (
	"log"
	"os"

	"github.com/Andamio-Platform/andamio-cli/cmd/build"
	"github.com/Andamio-Platform/andamio-cli/cmd/query"
	"github.com/Andamio-Platform/andamio-cli/cmd/sync"
	"github.com/Andamio-Platform/andamio-cli/cmd/transaction"
	"github.com/Andamio-Platform/andamio-cli/cmd/write"
	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "andamio-cli",
	Short: "Command-line utilities for Andamio",
	Long: `
Roadmap Phases:
1. Utilities
2. Queries
3. Deployment

Learn more at andamio.io/blog/014
	
	`,
	// Uncomment the following line if your bare application
	// has an action associated with it:
	// Run: func(cmd *cobra.Command, args []string) { },
}

var docCmd = &cobra.Command{
	Use:   "doc",
	Short: "Generate andamio-cli documentation",
	Long: `
Generate markdown docs in ./docs	
	`,
	Run: func(cmd *cobra.Command, args []string) {
		err := doc.GenMarkdownTree(rootCmd, "./docs")
		if err != nil {
			log.Fatal(err)
		}
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func addSubcommandIslands() {
	rootCmd.AddCommand(write.WriteCmd)
	rootCmd.AddCommand(query.QueryCmd)
	rootCmd.AddCommand(transaction.TransactionCmd)
	rootCmd.AddCommand(sync.SyncCmd)
	rootCmd.AddCommand(build.BuildCmd)
	rootCmd.AddCommand(docCmd)
}

func init() {
	addSubcommandIslands()
	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	// rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.andamio-cli.yaml)")

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
