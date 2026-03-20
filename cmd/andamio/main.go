package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/Andamio-Platform/andamio-cli/internal/apierr"
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
	rootCmd.SilenceErrors = true
	rootCmd.SilenceUsage = true
}

// Exit codes:
//
//	0 — success
//	1 — generic error (network, server, unexpected)
//	2 — not found (resource doesn't exist)
//	3 — auth required (no API key or JWT, or 401/403 response)
func main() {
	if err := rootCmd.Execute(); err != nil {
		exitCode := 1

		// ReportedError means the command already printed structured output to
		// stdout (e.g., a JSON RunResult). Unwrap for exit code mapping but
		// skip printing a second error message.
		var reported *apierr.ReportedError
		alreadyReported := errors.As(err, &reported)
		if alreadyReported {
			err = reported.Err
		}

		var notFound *apierr.NotFoundError
		var authErr *apierr.AuthError
		switch {
		case errors.As(err, &notFound):
			exitCode = 2
		case errors.As(err, &authErr):
			exitCode = 3
		}

		if !alreadyReported {
			if output.GetFormat() == output.FormatJSON {
				b, _ := json.Marshal(map[string]string{"error": err.Error()})
				fmt.Println(string(b))
			} else {
				fmt.Fprintln(os.Stderr, err)
			}
		}
		os.Exit(exitCode)
	}
}
