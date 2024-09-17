/*
Copyright © 2024 Andamio dev@andamio.io
*/
package cmd

import (
	"os"

	"github.com/Andamio-Platform/andamio-cli/cmd/query"
	"github.com/Andamio-Platform/andamio-cli/cmd/sync"
	"github.com/Andamio-Platform/andamio-cli/cmd/transaction"
	"github.com/Andamio-Platform/andamio-cli/cmd/write"
	"github.com/spf13/cobra"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "andamio-cli",
	Short: "Instant access to Andamio",
	Long: `
  Welcome to Andamio CLI. With this program, you can:
  1. Query the Andamio Network
  2. Write and convert data for Andamio transactions
  3. Build transactions to interact with Andamio

  Learn More: https://andamio.io
	
	`,

	// Uncomment the following line if your bare application
	// has an action associated with it:
	// Run: func(cmd *cobra.Command, args []string) { },
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
	rootCmd.AddCommand(docCmd)
}

func init() {
	rootCmd.CompletionOptions.DisableDefaultCmd = true
	addSubcommandIslands()
	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	// rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.andamio-cli.yaml)")

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
