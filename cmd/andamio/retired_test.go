package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os/exec"
	"path/filepath"
	"strconv"
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

// --- 1.0 surface assertions (issue #129) -----------------------------------

// resolve walks the real rootCmd to the command a path resolves to, returning
// the command and whether every segment matched exactly. A retired path must
// resolve to a registered stub — not fall through to cobra's unknown-command
// handling, which is the generic error issue #123 rules out.
func resolve(path string) (*cobra.Command, bool) {
	cmd := rootCmd
	for _, segment := range strings.Fields(path) {
		child := findChild(cmd, segment)
		if child == nil {
			return cmd, false
		}
		cmd = child
	}
	return cmd, true
}

// Every retired path resolves to a hidden stub. This is the guard that keeps a
// future refactor from re-registering `course student` as a working command
// while the registry still claims it is gone.
func TestRetiredPaths_ResolveToHiddenStubs(t *testing.T) {
	for _, entry := range retiredCommands {
		t.Run(entry.Path, func(t *testing.T) {
			cmd, ok := resolve(entry.Path)
			if !ok {
				t.Fatalf("%q does not resolve to a registered command; it would hit cobra's unknown-command error", entry.Path)
			}
			if !cmd.Hidden {
				t.Errorf("%q resolves to a visible command; retired paths must not appear in help", entry.Path)
			}
			if cmd.RunE == nil {
				t.Fatalf("%q has no RunE; it would print help and exit 0", entry.Path)
			}
			err := cmd.RunE(cmd, nil)
			var removed *apierr.RemovedCommandError
			if !errors.As(err, &removed) {
				t.Errorf("%q returned %v, want *apierr.RemovedCommandError", entry.Path, err)
			}
		})
	}
}

// The complement of the above: walking the live tree must not surface a
// retired name anywhere a user could find it.
func TestCommandTree_ExposesNoRetiredCommands(t *testing.T) {
	retired := make(map[string]bool, len(retiredCommands))
	for _, entry := range retiredCommands {
		retired[entry.Path] = true
	}

	var walk func(cmd *cobra.Command, path []string)
	walk = func(cmd *cobra.Command, path []string) {
		for _, child := range cmd.Commands() {
			childPath := append(append([]string{}, path...), child.Name())
			joined := strings.Join(childPath, " ")
			if retired[joined] && !child.Hidden {
				t.Errorf("%q is registered as a visible command but the registry says it was retired", joined)
			}
			if !child.Hidden {
				walk(child, childPath)
			}
		}
	}
	walk(rootCmd, nil)
}

// The removal has to be visible in help output, which is what a user actually
// reads. Checking the rendered text catches a stub that is registered but not
// Hidden, which the tree walk above would also catch — belt and braces on the
// surface issue #127 turns on.
func TestHelpOutput_OmitsRetiredGroups(t *testing.T) {
	cases := []struct{ group, retired string }{
		{"course", "student"},
		{"project", "contributor"},
	}

	for _, tc := range cases {
		t.Run(tc.group, func(t *testing.T) {
			cmd, ok := resolve(tc.group)
			if !ok {
				t.Fatalf("%q is not registered", tc.group)
			}
			var buf strings.Builder
			cmd.SetOut(&buf)
			if err := cmd.Usage(); err != nil {
				t.Fatalf("Usage: %v", err)
			}
			if strings.Contains(buf.String(), tc.retired) {
				t.Errorf("`andamio %s --help` still lists %q:\n%s", tc.group, tc.retired, buf.String())
			}
		})
	}
}

// No source file may call an API route belonging to the retired surface.
// Removing the commands without removing their route calls would leave dead
// requests firing from shared code paths — which is exactly what
// lookupContributorTaskHash was doing inside the tx lifecycle before 1.0.
//
// Inspects string literals rather than raw file text, deliberately. A textual
// scan also matches comments, and the comments explaining *why* a route was
// removed are worth keeping — a guard that forbids documenting the removal
// would push maintainers toward deleting the explanation instead of the code.
//
// Skips _test.go files, which legitimately name these routes in order to
// assert they are never called.
func TestNoSourceFileCallsARetiredRoute(t *testing.T) {
	forbidden := []string{
		"/course/student/",
		"/project/contributor/",
	}

	for _, root := range []string{"../../cmd", "../../internal"} {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}

			file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			if err != nil {
				return fmt.Errorf("parsing %s: %w", path, err)
			}

			ast.Inspect(file, func(n ast.Node) bool {
				lit, ok := n.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					return true
				}
				value, err := strconv.Unquote(lit.Value)
				if err != nil {
					return true
				}
				for _, route := range forbidden {
					if strings.Contains(value, route) {
						t.Errorf("%s has a string literal referencing retired API route %q: %s", path, route, lit.Value)
					}
				}
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatalf("walking %s: %v", root, err)
		}
	}
}

// --- end-to-end behavior through the real binary --------------------------

// runRetired invokes the built binary and returns stdout, stderr and the exit
// code. A retired command is expected to fail, so a non-zero exit is the normal
// path here rather than a test failure.
func runRetired(t *testing.T, bin string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err := cmd.Run()
	var exitErr *exec.ExitError
	switch {
	case err == nil:
		exitCode = 0
	case errors.As(err, &exitErr):
		exitCode = exitErr.ExitCode()
	default:
		t.Fatalf("running %v: %v", args, err)
	}
	return outBuf.String(), errBuf.String(), exitCode
}

// The behavior issue #123 actually specifies, exercised through the binary a
// user runs. The pre-1.0 baseline for the bare-group case was help output on
// stdout with exit 0 — verified by running the previous build — so the exit
// code assertion is a real regression guard, not a tautology.
func TestRetiredCommand_EndToEnd_TextMode(t *testing.T) {
	bin := buildTestBinary(t)

	cases := []struct {
		name string
		args []string
		want string
	}{
		{"bare group", []string{"course", "student"}, "course student"},
		{"subcommand", []string{"course", "student", "claim"}, "course student claim"},
		{"subcommand with retired flags", []string{"course", "student", "submit", "--course-id", "abc", "--evidence", "x"}, "course student submit"},
		{"contributor group", []string{"project", "contributor"}, "project contributor"},
		{"contributor subcommand with flags", []string{"project", "contributor", "commit", "--project-id", "p", "--task-index", "3"}, "project contributor commit"},
		{"unrecognized subcommand", []string{"course", "student", "bogus", "extra"}, "course student"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, code := runRetired(t, bin, tc.args...)

			if code != 4 {
				t.Errorf("exit code = %d, want 4 (removed command)", code)
			}
			if !strings.Contains(stderr, tc.want) {
				t.Errorf("stderr does not name %q:\n%s", tc.want, stderr)
			}
			if !strings.Contains(stderr, "https://app.andamio.io") {
				t.Errorf("stderr does not name the alternative:\n%s", stderr)
			}
			if strings.Contains(stderr, "Usage:") || strings.Contains(stdout, "Usage:") {
				t.Errorf("retired command printed usage/help; it must explain the removal instead:\n%s%s", stdout, stderr)
			}
			if stdout != "" {
				t.Errorf("retired command wrote to stdout (%q); errors belong on stderr", stdout)
			}
		})
	}
}

// A caller that asked for JSON gets JSON, even when the answer is "this command
// is gone". This is the regression guard on the FParseErrWhitelist choice in
// newRetiredCommand: swapping back to DisableFlagParsing would skip parsing of
// the root's persistent --output flag and silently drop this to plain text.
func TestRetiredCommand_EndToEnd_JSONMode(t *testing.T) {
	bin := buildTestBinary(t)

	for _, args := range [][]string{
		{"course", "student", "claim", "--output", "json"},
		{"project", "contributor", "commit", "-o", "json", "--project-id", "p"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			stdout, _, code := runRetired(t, bin, args...)

			if code != 4 {
				t.Errorf("exit code = %d, want 4", code)
			}
			var parsed map[string]string
			if err := json.Unmarshal([]byte(stdout), &parsed); err != nil {
				t.Fatalf("--output json did not emit JSON on stdout: %v\nraw: %q", err, stdout)
			}
			msg, ok := parsed["error"]
			if !ok {
				t.Fatalf("JSON envelope has no \"error\" key: %v", parsed)
			}
			if !strings.Contains(msg, "https://app.andamio.io") {
				t.Errorf("JSON error message does not name the alternative: %q", msg)
			}
		})
	}
}

// The removal must not have taken the 1.0 surface with it. If deleting the
// learner and contributor files knocked out a sibling group, this is where it
// shows up.
func TestCommandTree_PreservesTheOneZeroSurface(t *testing.T) {
	survivors := []string{
		"course owner", "course teacher", "course credential",
		"course list", "course modules", "course slts",
		"project owner", "project manager", "project task",
		"teacher assignments", "manager",
		"tx build", "tx sign", "tx submit", "tx register", "tx run",
		"user login", "user me", "dev login", "auth login", "config show",
	}

	for _, path := range survivors {
		cmd, ok := resolve(path)
		if !ok {
			t.Errorf("%q is no longer registered; the 1.0 surface lost a command it should keep", path)
			continue
		}
		if cmd.Hidden {
			t.Errorf("%q is hidden; it belongs to the 1.0 surface", path)
		}
	}
}
