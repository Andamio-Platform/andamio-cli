package main

import (
	"fmt"
	"os"

	"github.com/Andamio-Platform/andamio-cli/internal/output"
	"github.com/spf13/cobra"
)

var (
	version      = "dev"
	commit       = "none"
	date         = "unknown"
	outputFormat string
)

func versionString() string {
	if commit == "none" {
		return version
	}
	return version + " (" + commit[:7] + " " + date + ")"
}

var rootCmd = &cobra.Command{
	Use:     "andamio",
	Short:   "CLI for interacting with the Andamio Protocol",
	Version: versionString(),
	Long: `Andamio CLI provides commands for interacting with the Andamio Protocol.

Query courses, credentials, and more from the command line.`,
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
