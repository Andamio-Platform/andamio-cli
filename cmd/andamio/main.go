package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "andamio",
	Short: "CLI for interacting with the Andamio Protocol",
	Long: `Andamio CLI provides commands for interacting with the Andamio Protocol.

Query courses, credentials, and more from the command line.`,
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
