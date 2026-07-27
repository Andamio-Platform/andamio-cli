package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Andamio-Platform/andamio-cli/internal/apierr"
	"github.com/spf13/cobra"
)

// Andamio CLI 1.0 scoped the tool to the Owner, Teacher and Manager roles —
// the roles that author work and assess it. The learner and contributor command
// surface was removed (issue #129).
//
// Removing the commands outright would leave anyone following a pre-1.0
// tutorial, script or README staring at a generic "unknown command" error, or —
// worse — at the group's help text with exit code 0, which is what the CLI
// actually did before this change. Issue #123 asks for something better: a
// removed command should say what happened and where the operation lives now.
//
// This file reconciles the two. Retired commands stay *registered* but are
// hidden from help, carry no implementation, and fail with a typed error that
// names the removal and its replacement. "Removed" here means invisible and
// non-functional, not unroutable.
//
// One registry drives all of it: the stub registration, the message text, and
// the regression tests that keep the retired names from quietly coming back.
// Adding a future retirement is one table entry, not three edits.

// retiredAppGuidance is the replacement guidance for the learner and
// contributor surface retired in 1.0. Both groups point at the same place, so
// they share one string — but guidance is per-entry so a future retirement can
// point somewhere else without restructuring the registry.
const retiredAppGuidance = "Learner and contributor work now happens in the Andamio app, " +
	"which signs and submits it in one flow: https://app.andamio.io"

// retiredCommand describes one command path retired from the CLI surface.
//
// Path is the full invocation minus the "andamio" binary name, space-separated
// (e.g. "course student submit"). Every segment except the last must already
// exist — either as a live command (like "course") or as an earlier registry
// entry (like "course student"). registerRetiredCommands enforces this.
//
// Guidance is the sentence shown after the removal notice. It should name where
// the operation lives now, not merely state that it is gone.
type retiredCommand struct {
	Path     string
	Guidance string
}

// retiredCommands is the single source of truth for what was retired.
//
// Two groups and their fifteen subcommands, matching what
// cmd/andamio/course_student.go and cmd/andamio/project_contributor.go
// registered before they were deleted. The group entries matter as much as the
// leaves: without them, `andamio course student` alone would fall through to
// cobra's unknown-command error instead of explaining itself.
var retiredCommands = []retiredCommand{
	// course student — removed in 1.0 (issue #129)
	{Path: "course student", Guidance: retiredAppGuidance},
	{Path: "course student courses", Guidance: retiredAppGuidance},
	{Path: "course student credentials", Guidance: retiredAppGuidance},
	{Path: "course student commitments", Guidance: retiredAppGuidance},
	{Path: "course student commitment", Guidance: retiredAppGuidance},
	{Path: "course student create", Guidance: retiredAppGuidance},
	{Path: "course student submit", Guidance: retiredAppGuidance},
	{Path: "course student update", Guidance: retiredAppGuidance},
	{Path: "course student leave", Guidance: retiredAppGuidance},
	{Path: "course student claim", Guidance: retiredAppGuidance},

	// project contributor — removed in 1.0 (issue #129)
	{Path: "project contributor", Guidance: retiredAppGuidance},
	{Path: "project contributor list", Guidance: retiredAppGuidance},
	{Path: "project contributor commitments", Guidance: retiredAppGuidance},
	{Path: "project contributor commitment", Guidance: retiredAppGuidance},
	{Path: "project contributor commit", Guidance: retiredAppGuidance},
	{Path: "project contributor update", Guidance: retiredAppGuidance},
	{Path: "project contributor delete", Guidance: retiredAppGuidance},
}

func init() {
	if err := registerRetiredCommands(rootCmd, retiredCommands); err != nil {
		// Unreachable in a correct build: the only failure modes are a malformed
		// registry entry or a collision with a live command, both of which are
		// programming errors pinned by TestRegisterRetiredCommands. Failing loudly
		// here beats silently dropping a stub and regressing issue #123.
		panic(fmt.Sprintf("retired command registry is invalid: %v", err))
	}
}

// registerRetiredCommands attaches a hidden stub for every entry in the
// registry. Entries are processed shortest-path-first so a group ("course
// student") is registered before the subcommands that hang off it.
//
// Returns an error rather than registering partially: a retired path that
// collides with a live command means the deletion was incomplete, and shipping
// that silently would hand the user a working command the CLI claims is gone.
func registerRetiredCommands(root *cobra.Command, entries []retiredCommand) error {
	sorted := make([]retiredCommand, len(entries))
	copy(sorted, entries)
	sort.SliceStable(sorted, func(i, j int) bool {
		return len(strings.Fields(sorted[i].Path)) < len(strings.Fields(sorted[j].Path))
	})

	seen := make(map[string]bool, len(sorted))

	for _, entry := range sorted {
		segments := strings.Fields(entry.Path)
		if len(segments) == 0 {
			return fmt.Errorf("registry entry has an empty path")
		}
		if entry.Guidance == "" {
			return fmt.Errorf("retired command %q has no replacement guidance", entry.Path)
		}
		if seen[entry.Path] {
			return fmt.Errorf("retired command %q is registered twice", entry.Path)
		}
		seen[entry.Path] = true

		parent := root
		for _, segment := range segments[:len(segments)-1] {
			child := findChild(parent, segment)
			if child == nil {
				return fmt.Errorf("retired command %q: no command named %q to attach to", entry.Path, segment)
			}
			parent = child
		}

		leaf := segments[len(segments)-1]
		if existing := findChild(parent, leaf); existing != nil {
			return fmt.Errorf("retired command %q is still registered as a live command", entry.Path)
		}

		parent.AddCommand(newRetiredCommand(leaf, entry))
	}

	return nil
}

// newRetiredCommand builds the hidden stub for one retired path.
//
// Three settings matter, and each closes a way the removal message could fail
// to reach the user:
//
//   - Hidden keeps the command out of `--help`, so the 1.0 surface reads as a
//     coherent tool rather than one with holes in it (issue #127).
//   - FParseErrWhitelist.UnknownFlags means `course student submit --course-id x`
//     reaches RunE instead of dying on an unknown flag. Anyone running a retired
//     command is following pre-1.0 instructions, and those instructions carry
//     flags that no longer exist.
//   - ArbitraryArgs means trailing positional arguments don't trigger a cobra
//     usage error before RunE runs.
//
// The whitelist is deliberately used in preference to DisableFlagParsing, which
// would also get the message through. DisableFlagParsing skips *all* parsing,
// including the root's persistent --output flag, so `course student claim
// --output json` would print a plain-text error and never reach the JSON error
// envelope in main.go. A caller that asked for JSON gets JSON, even when the
// answer is "this command is gone".
//
// The stub deliberately does not inherit an auth PersistentPreRunE. The groups
// this replaces both gated on jwtAuthPreRunE, and inheriting that would greet an
// unauthenticated caller with "not authenticated. Run 'andamio user login'
// first" — technically true, entirely unhelpful, and not the message issue #123
// asks for.
func newRetiredCommand(use string, entry retiredCommand) *cobra.Command {
	return &cobra.Command{
		Use:    use,
		Hidden: true,
		Args:   cobra.ArbitraryArgs,
		FParseErrWhitelist: cobra.FParseErrWhitelist{
			UnknownFlags: true,
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return &apierr.RemovedCommandError{
				Command:  entry.Path,
				Guidance: entry.Guidance,
			}
		},
	}
}

// findChild returns the direct child of parent registered under name, matching
// on the command's name (the first word of Use) and on its aliases. Returns nil
// when no such child exists.
func findChild(parent *cobra.Command, name string) *cobra.Command {
	for _, c := range parent.Commands() {
		if c.Name() == name {
			return c
		}
		for _, alias := range c.Aliases {
			if alias == name {
				return c
			}
		}
	}
	return nil
}
