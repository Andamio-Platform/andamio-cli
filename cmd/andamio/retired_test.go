package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/Andamio-Platform/andamio-cli/internal/apierr"
	"github.com/spf13/cobra"
)

// newTestTree builds a throwaway command tree shaped like the real one —
// a root with a live "course" group — so registry behavior can be exercised
// without mutating rootCmd, which is package-global and already populated by
// every init() in the package.
func newTestTree() *cobra.Command {
	root := &cobra.Command{Use: "andamio"}
	root.AddCommand(&cobra.Command{Use: "course"})
	return root
}

func TestNewRetiredCommand_ReturnsTypedError(t *testing.T) {
	entry := retiredCommand{Path: "course student submit", Guidance: retiredAppGuidance}
	cmd := newRetiredCommand("submit", entry)

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("retired stub returned nil error; it must always fail")
	}

	var removed *apierr.RemovedCommandError
	if !errors.As(err, &removed) {
		t.Fatalf("error is %T, want *apierr.RemovedCommandError", err)
	}
	if removed.Command != "course student submit" {
		t.Errorf("Command = %q, want %q", removed.Command, "course student submit")
	}
}

// The message has to carry both halves of what issue #123 asks for: what
// happened, and where the operation lives now. Either alone is a worse error
// than the generic one it replaces.
func TestRemovedCommandError_MessageNamesCommandAndAlternative(t *testing.T) {
	err := &apierr.RemovedCommandError{
		Command:  "project contributor commit",
		Guidance: retiredAppGuidance,
	}
	msg := err.Error()

	for _, want := range []string{"project contributor commit", "1.0", "https://app.andamio.io"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message missing %q\ngot: %s", want, msg)
		}
	}
}

// Anyone running a retired command is following pre-1.0 instructions, and those
// instructions carry flags. Without DisableFlagParsing the caller gets "unknown
// flag: --course-id" instead of the removal notice.
func TestRetiredCommand_AcceptsUnknownFlagsAndArgs(t *testing.T) {
	root := newTestTree()
	entries := []retiredCommand{
		{Path: "course student", Guidance: retiredAppGuidance},
		{Path: "course student submit", Guidance: retiredAppGuidance},
	}
	if err := registerRetiredCommands(root, entries); err != nil {
		t.Fatalf("registerRetiredCommands: %v", err)
	}

	cases := []struct {
		name string
		args []string
		want string
	}{
		{"bare group", []string{"course", "student"}, "course student"},
		{"subcommand", []string{"course", "student", "submit"}, "course student submit"},
		{"subcommand with flags", []string{"course", "student", "submit", "--course-id", "abc", "--evidence", "x"}, "course student submit"},
		{"subcommand with trailing args", []string{"course", "student", "submit", "extra", "args"}, "course student submit"},
		{"unrecognized subcommand falls back to the group", []string{"course", "student", "bogus"}, "course student"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root.SetArgs(tc.args)
			root.SetOut(&strings.Builder{})
			root.SetErr(&strings.Builder{})
			root.SilenceErrors = true
			root.SilenceUsage = true

			err := root.Execute()
			if err == nil {
				t.Fatal("expected a removal error, got nil (the pre-1.0 behavior was help + exit 0)")
			}
			var removed *apierr.RemovedCommandError
			if !errors.As(err, &removed) {
				t.Fatalf("error is %T (%v), want *apierr.RemovedCommandError", err, err)
			}
			if removed.Command != tc.want {
				t.Errorf("Command = %q, want %q", removed.Command, tc.want)
			}
		})
	}
}

func TestRetiredCommand_IsHidden(t *testing.T) {
	root := newTestTree()
	entries := []retiredCommand{{Path: "course student", Guidance: retiredAppGuidance}}
	if err := registerRetiredCommands(root, entries); err != nil {
		t.Fatalf("registerRetiredCommands: %v", err)
	}

	stub := findChild(findChild(root, "course"), "student")
	if stub == nil {
		t.Fatal("retired stub was not registered")
	}
	if !stub.Hidden {
		t.Error("retired stub is visible in help output; the 1.0 surface must not advertise it")
	}
}

func TestRegisterRetiredCommands_RejectsBadEntries(t *testing.T) {
	cases := []struct {
		name    string
		entries []retiredCommand
		wantErr string
	}{
		{
			name:    "empty path",
			entries: []retiredCommand{{Path: "", Guidance: retiredAppGuidance}},
			wantErr: "empty path",
		},
		{
			name:    "missing guidance",
			entries: []retiredCommand{{Path: "course student", Guidance: ""}},
			wantErr: "no replacement guidance",
		},
		{
			name: "duplicate path",
			entries: []retiredCommand{
				{Path: "course student", Guidance: retiredAppGuidance},
				{Path: "course student", Guidance: retiredAppGuidance},
			},
			wantErr: "registered twice",
		},
		{
			name:    "orphan parent",
			entries: []retiredCommand{{Path: "nonexistent student", Guidance: retiredAppGuidance}},
			wantErr: "no command named",
		},
		{
			name:    "collides with a live command",
			entries: []retiredCommand{{Path: "course", Guidance: retiredAppGuidance}},
			wantErr: "still registered as a live command",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := registerRetiredCommands(newTestTree(), tc.entries)
			if err == nil {
				t.Fatal("expected an error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

// Entries are registered shortest-path-first so a group exists before the
// subcommands hanging off it. Feeding them in reverse order must still work —
// registry order is a maintenance detail, not a contract.
func TestRegisterRetiredCommands_OrderIndependent(t *testing.T) {
	root := newTestTree()
	entries := []retiredCommand{
		{Path: "course student submit", Guidance: retiredAppGuidance},
		{Path: "course student", Guidance: retiredAppGuidance},
	}
	if err := registerRetiredCommands(root, entries); err != nil {
		t.Fatalf("registerRetiredCommands with child-before-parent ordering: %v", err)
	}

	student := findChild(findChild(root, "course"), "student")
	if student == nil {
		t.Fatal("group stub was not registered")
	}
	if findChild(student, "submit") == nil {
		t.Error("subcommand stub was not registered under the group stub")
	}
}

// The real registry has to survive the real command tree. init() panics on
// failure, so this is belt-and-braces — but it localizes the diagnosis to this
// test instead of every test in the package failing at init.
func TestRegisterRetiredCommands_RealRegistryIsValid(t *testing.T) {
	for _, entry := range retiredCommands {
		if strings.TrimSpace(entry.Path) == "" {
			t.Error("registry contains an entry with an empty path")
		}
		if entry.Guidance == "" {
			t.Errorf("retired command %q has no replacement guidance", entry.Path)
		}
	}
}
