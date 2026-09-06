package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"

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
// directly). Case-insensitive JSON match so `--output JSON` works the same as `--output
// json`, matching how other commands handle --output (normalized inside SetFormat).
// Non-"json" values silently fall through to text — documented in --help; strict
// validation would require re-plumbing Cobra's template path through a normal RunE.
func buildVersionOutput() string {
	if strings.EqualFold(outputFormat, "json") {
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
	Short:   "Developer CLI for authoring and assessing on the Andamio Protocol",
	Version: versionString(),
	Long: `Andamio CLI is a developer tool for the people who author work and assess it:
course Owners and Teachers, and project Managers.

  Own       create courses and projects, register them on-chain, manage
            teachers and managers
  Teach     write and publish module content, review submissions, build
            assessment transactions
  Manage    create and mint project tasks, review contributions, assess work

Learners and contributors use the Andamio app, which signs and submits their
work in one flow.

BUILT TO BE DRIVEN BY PROGRAMS

Every list and get command takes --output json, and that is the stable surface
scripts and agents should use. Progress goes to stderr, data to stdout, and no
command reads stdin or prompts — everything works without a TTY.

Failures are distinguishable without parsing prose: each carries an exit code
and a "kind" field. An empty result is a success (exit 0, empty collection),
which is what keeps "nothing found" apart from "not permitted" (exit 3) and
"could not reach the service" (exit 5). Run 'andamio help exit-codes' for the
full table.

"andamio --version --output json" emits {"version":"<x>","commit":"<sha7>",
"built":"<timestamp>"} so a caller can identify the CLI before invoking it.
See CHANGELOG.md for the envelope contract and breaking-change history.`,
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

// Exit codes, paired with the "kind" field of the --output json error envelope:
//
//	0                     — success, including an empty but valid result set
//	1  error              — generic: unexpected, server-side, interrupted, bad input
//	1  verify             — a write was accepted but its read-back did not confirm
//	                        the stored value (course import-assignment)
//	2  not_found          — resource doesn't exist (404)
//	3  auth               — no credentials, or 401/403
//	4  removed_command    — retired in 1.0 (see cmd/andamio/retired.go)
//	5  unreachable        — the request never reached the service
//	6  conflict           — conflicts with existing state (409)
//	7  tier_limit         — the account's plan does not permit this action;
//	                        remedy is billing-side (revoke, upgrade, subscribe)
//
// A caller can branch on the exit code alone or on "kind" alone; the two never
// disagree. Kinds without a dedicated exit code (server, backpressure,
// canceled, verify) share exit 1 — they are already distinguishable via "kind",
// and splitting them further buys a caller nothing today.
//
// Codes 0-3 predate 1.0 and are load-bearing for existing scripts, so they are
// fixed. 4, 5 and 7 are new. 6 is the one change to an existing path: conflicts
// were typed but exited 1, indistinguishable from a generic failure. 1.0 is
// the right moment to fix that, and it is called out in the changelog. 7 is
// classified by the gateway's body code (tier_limit_exceeded), not by HTTP
// status: the cap rides on 429 today and is ruled to move to 403, and a
// caller told "retry later" or "re-login" for a plan cap would act on the
// wrong remedy either way.
//
// Note that an empty result is NOT an error. Commands that find nothing emit
// an empty collection and exit 0 — "nothing found", "could not reach the
// service" and "not permitted" stay three distinct outcomes.
func main() {
	// Wire SIGINT to the cobra context so Ctrl-C cancels cmd.Context() in
	// every subcommand. Individual commands that install their own signal
	// handler (e.g., tx_lifecycle.go) still work — Go multiplexes signals
	// to all registered channels. A second Ctrl-C falls through to the
	// default runtime handler and terminates immediately.
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	// Runs here rather than in an init() because the command tree is only
	// complete once every file's init() has registered its commands — see
	// guardUnknownSubcommands.
	guardUnknownSubcommands(rootCmd)

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		exitCode := 1

		// ReportedError means the command already printed structured output to
		// stdout (e.g., a JSON RunResult). Unwrap for exit code mapping but
		// skip printing a second error message.
		var reported *apierr.ReportedError
		alreadyReported := errors.As(err, &reported)
		if alreadyReported {
			err = reported.Err
		}

		// Exit code and kind are derived from the same classification so they
		// can never disagree — apierr.Kind is the single mapper, and this
		// switch translates its result rather than re-inspecting the error.
		kind := apierr.Kind(err)
		switch kind {
		case apierr.KindNotFound:
			exitCode = 2
		case apierr.KindAuth:
			exitCode = 3
		case apierr.KindRemovedCommand:
			exitCode = 4
		case apierr.KindUnreachable:
			exitCode = 5
		case apierr.KindConflict:
			exitCode = 6
		case apierr.KindTierLimit:
			exitCode = 7
		}

		if !alreadyReported {
			if output.GetFormat() == output.FormatJSON {
				b, _ := json.Marshal(map[string]string{
					"error": err.Error(),
					"kind":  kind,
				})
				fmt.Println(string(b))
			} else {
				// Text mode is unchanged: no kind leaks into human output.
				fmt.Fprintln(os.Stderr, err)
			}
		}
		os.Exit(exitCode)
	}
}
