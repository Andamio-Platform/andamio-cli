package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// Cobra's default for a command that has subcommands but no Run is to print
// help and return nil — exit 0. That is correct for `andamio course` with no
// arguments (the user asked what is available), and wrong for `andamio course
// lst`, which is a typo the caller needs to hear about.
//
// Before this, both did the same thing: 56 lines of help on **stdout** and exit
// 0. A script doing
//
//	andamio course lst --output json | jq '.data[]'
//
// got help text in the pipe and a success code — the single-return-value-for-
// two-outcomes failure issue #126 exists to remove, on the path a typo actually
// takes.
//
// guardUnknownSubcommands walks the tree and, for every group command, installs
// a RunE that errors on an unrecognized argument and otherwise falls back to
// help. Commands that already define Run or RunE are left alone, which is what
// keeps the retired-command stubs (and every real leaf command) untouched.

// guardUnknownSubcommands applies the guard to cmd and, recursively, to every
// subcommand that is itself a group.
//
// Called from main(), NOT from an init(). Go runs init() functions in filename
// order, so an init() here would walk a half-built tree: every command
// registered by a file sorting after "unknown.go" (user.go, tx*.go, spec.go...)
// would not exist yet and would silently keep the exit-0 behavior. main() runs
// after every init has completed, which is the only point where the tree is
// whole.
func guardUnknownSubcommands(cmd *cobra.Command) {
	for _, child := range cmd.Commands() {
		guardUnknownSubcommands(child)
	}

	// Leaf commands, and groups that define their own behavior, are none of
	// this function's business.
	if len(cmd.Commands()) == 0 || cmd.Run != nil || cmd.RunE != nil {
		return
	}

	// Cobra only defaults SuggestionsMinimumDistance to 2 inside its own
	// unknown-command path; a direct SuggestionsFor call sees the zero value
	// and can never match anything, since a Levenshtein distance of 0 means
	// the name was typed correctly. Set it explicitly or "lst" never suggests
	// "list".
	if cmd.SuggestionsMinimumDistance <= 0 {
		cmd.SuggestionsMinimumDistance = 2
	}

	cmd.RunE = func(c *cobra.Command, args []string) error {
		if len(args) == 0 {
			// Asking a group what it offers is a legitimate request. Help goes
			// to stdout and the command succeeds, as before.
			return c.Help()
		}
		return unknownSubcommandError(c, args[0])
	}
}

// unknownSubcommandError names what was not recognized and where to look, and
// suggests a near match when cobra can find one.
//
// Deliberately an error rather than a help dump: it travels to stderr, leaves
// stdout clean for whatever is downstream in the pipe, and exits non-zero.
func unknownSubcommandError(cmd *cobra.Command, arg string) error {
	path := cmd.CommandPath()

	var b strings.Builder
	fmt.Fprintf(&b, "unknown command %q for %q", arg, path)

	if suggestions := cmd.SuggestionsFor(arg); len(suggestions) > 0 {
		fmt.Fprintf(&b, "\n\nDid you mean this?\n")
		for _, s := range suggestions {
			fmt.Fprintf(&b, "\t%s %s\n", path, s)
		}
	}
	fmt.Fprintf(&b, "\nRun '%s --help' for available commands.", path)

	return fmt.Errorf("%s", b.String())
}
