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

// shortCommit returns the first 7 characters of commit, or commit unchanged when shorter.
// Guards against compile-time defaults like "none" (4 chars) that would panic on commit[:7].
func shortCommit(commit string) string {
	if len(commit) < 7 {
		return commit
	}
	return commit[:7]
}

func versionString() string {
	if commit == "none" {
		return version
	}
	return version + " (" + shortCommit(commit) + " " + date + ")"
}

// buildVersionOutput returns the string that --version prints. Branches on the --output
// flag: JSON-mode emits {version, commit, built}; otherwise the existing text format.
// Called by the Cobra version template via the "versionOutput" template function, which
// runs after flag parsing populates the outputFormat package var (cobra's --version path
// bypasses PersistentPreRunE, so output.SetFormat is never called — we read outputFormat
// directly).
func buildVersionOutput() string {
	if output.Format(outputFormat) == output.FormatJSON {
		payload := struct {
			Version string `json:"version"`
			Commit  string `json:"commit"`
			Built   string `json:"built"`
		}{
			Version: version,
			Commit:  shortCommit(commit),
			Built:   date,
		}
		b, _ := json.Marshal(payload)
		return string(b) + "\n"
	}
	return fmt.Sprintf("andamio %s (commit: %s, built: %s)\n", version, shortCommit(commit), date)
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
	cobra.AddTemplateFunc("versionOutput", buildVersionOutput)
	rootCmd.SetVersionTemplate(`{{versionOutput}}`)
	rootCmd.SilenceErrors = true
	rootCmd.SilenceUsage = true
	// TraverseChildren lets Cobra parse persistent flags on the root command before
	// walking positional args. Without this, "andamio --version --output json" parses
	// --version as a bool, then tries to route "json" (the value of --output) as a
	// subcommand — producing "unknown command 'json'". With TraverseChildren, --output
	// is consumed correctly regardless of its position relative to --version.
	rootCmd.TraverseChildren = true
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
