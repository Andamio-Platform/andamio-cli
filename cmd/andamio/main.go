package main

import (
	"fmt"
	"os"

	"github.com/Andamio-Platform/andamio-cli/internal/output"
	"github.com/spf13/cobra"
)

var (
	outputFormat string

	// Set via ldflags at build time
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

var rootCmd = &cobra.Command{
	Use:     "andamio",
	Short:   "CLI for interacting with the Andamio Protocol",
	Long:    `Andamio CLI provides commands for interacting with the Andamio Protocol.

Query courses, credentials, and more from the command line.`,
	Version: version,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		return output.SetFormat(outputFormat)
	},
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&outputFormat, "output", "o", "text", "Output format: text, json, csv, markdown")
	rootCmd.SetVersionTemplate(fmt.Sprintf("andamio %s (commit: %s, built: %s)\n", version, commit, date))
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
