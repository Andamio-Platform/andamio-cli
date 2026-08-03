package main

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// The defect this guards against: `andamio course lst --output json | jq ...`
// used to put 56 lines of help into the pipe and exit 0, so a script with a
// typo could not tell it had failed. Exercised through the built binary
// because the guard is applied in main(), which is also where the coverage
// gap below would reappear.
func TestUnknownCommand_ExitsNonZeroWithCleanStdout(t *testing.T) {
	bin := buildTestBinary(t)

	cases := []struct {
		name string
		args []string
		want string
	}{
		{"unknown root command", []string{"bogus-command"}, `unknown command "bogus-command"`},
		{"typo on a course subcommand", []string{"course", "lst"}, `unknown command "lst"`},
		{"typo on a user subcommand", []string{"user", "statu"}, `unknown command "statu"`},
		{"typo on a teacher subcommand", []string{"teacher", "bogus"}, `unknown command "bogus"`},
		{"typo on a tx subcommand", []string{"tx", "bogus"}, `unknown command "bogus"`},
		{"typo on a project subcommand", []string{"project", "bogus"}, `unknown command "bogus"`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, code := runRetired(t, bin, tc.args...)

			if code == 0 {
				t.Errorf("exit code = 0; an unrecognized command must not report success")
			}
			if stdout != "" {
				t.Errorf("stdout was polluted with %d bytes; it must stay clean for the pipe:\n%s", len(stdout), stdout)
			}
			if !strings.Contains(stderr, tc.want) {
				t.Errorf("stderr does not contain %q:\n%s", tc.want, stderr)
			}
		})
	}
}

// A near miss should say what was probably meant. Without an explicit
// SuggestionsMinimumDistance this silently never fires, since cobra only
// defaults it inside its own unknown-command path.
func TestUnknownCommand_SuggestsNearMatches(t *testing.T) {
	bin := buildTestBinary(t)

	cases := []struct {
		args []string
		want string
	}{
		{[]string{"course", "lst"}, "course list"},
		{[]string{"user", "statu"}, "user status"},
		{[]string{"course", "expor"}, "course export"},
		{[]string{"teacher", "assignment"}, "teacher assignments"},
	}

	for _, tc := range cases {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			_, stderr, _ := runRetired(t, bin, tc.args...)

			if !strings.Contains(stderr, "Did you mean") {
				t.Errorf("no suggestion offered for %v:\n%s", tc.args, stderr)
			}
			if !strings.Contains(stderr, tc.want) {
				t.Errorf("suggestion does not include %q:\n%s", tc.want, stderr)
			}
		})
	}
}

// Asking a group what it offers is not an error. This is the behavior the
// guard must preserve while fixing the typo case.
func TestUnknownCommand_BareGroupStillShowsHelp(t *testing.T) {
	bin := buildTestBinary(t)

	for _, args := range [][]string{{}, {"course"}, {"project"}, {"teacher"}, {"tx"}, {"user"}} {
		name := "root"
		if len(args) > 0 {
			name = args[0]
		}
		t.Run(name, func(t *testing.T) {
			stdout, _, code := runRetired(t, bin, args...)

			if code != 0 {
				t.Errorf("exit code = %d, want 0 — asking a group what it offers is legitimate", code)
			}
			if !strings.Contains(stdout, "Available Commands") {
				t.Errorf("no help on stdout:\n%s", stdout)
			}
		})
	}
}

// Groups that gate their subcommands on a user JWT must still describe
// themselves to a caller who has not logged in.
//
// The guard works by installing a RunE, and that makes a group Runnable. Cobra
// short-circuits non-runnable commands with flag.ErrHelp *before* it walks the
// PersistentPreRunE chain, so making these groups runnable started routing them
// through jwtAuthPreRunE for the first time. `andamio teacher` began answering
// "not authenticated" instead of listing its subcommands — a regression against
// pre-1.0, on the two role groups 1.0 is scoped to.
//
// runRetired supplies an empty HOME, so every case here runs with no
// credentials at all. That is the condition the regression needed.
func TestUnknownCommand_AuthGatedGroupsDescribeThemselvesUnauthenticated(t *testing.T) {
	bin := buildTestBinary(t)

	// Every group carrying jwtAuthPreRunE, directly or by inheritance.
	groups := [][]string{
		{"teacher"},
		{"manager"},
		{"course", "owner"},
		{"project", "owner"},
		{"project", "manager"},
		{"project", "task"},
	}

	for _, args := range groups {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			stdout, stderr, code := runRetired(t, bin, args...)

			if code != 0 {
				t.Errorf("exit code = %d, want 0 — listing subcommands must not require login.\nstderr: %s", code, stderr)
			}
			if !strings.Contains(stdout, "Available Commands") {
				t.Errorf("no help on stdout:\n%s", stdout)
			}
			if strings.Contains(stderr, "not authenticated") || strings.Contains(stderr, "session expired") {
				t.Errorf("auth was enforced on a help-only invocation:\n%s", stderr)
			}
		})
	}
}

// A typo under an auth-gated group must report the typo, not the login state.
//
// Same root cause as the test above, but the failure is worse: issue #126 exists
// so a caller can tell outcomes apart, and reporting exit 3 "not authenticated"
// for a misspelled subcommand sends them to re-authenticate over a problem that
// logging in cannot fix.
func TestUnknownCommand_TypoUnderAuthGatedGroupReportsTypo(t *testing.T) {
	bin := buildTestBinary(t)

	cases := []struct {
		args []string
		want string
	}{
		{[]string{"teacher", "bogus"}, `unknown command "bogus"`},
		{[]string{"teacher", "assignment"}, "teacher assignments"},
		{[]string{"manager", "bogus"}, `unknown command "bogus"`},
		{[]string{"course", "owner", "bogus"}, `unknown command "bogus"`},
		{[]string{"project", "task", "bogus"}, `unknown command "bogus"`},
	}

	for _, tc := range cases {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			stdout, stderr, code := runRetired(t, bin, tc.args...)

			if code == 0 {
				t.Errorf("exit code = 0; an unrecognized command must not report success")
			}
			if stdout != "" {
				t.Errorf("stdout was polluted with %d bytes; it must stay clean for the pipe:\n%s", len(stdout), stdout)
			}
			if !strings.Contains(stderr, tc.want) {
				t.Errorf("stderr does not contain %q:\n%s", tc.want, stderr)
			}
			if strings.Contains(stderr, "not authenticated") {
				t.Errorf("reported the login state instead of the typo:\n%s", stderr)
			}
		})
	}
}

// Retired commands have their own RunE and must keep their own exit code —
// the guard skips any command that already defines behavior.
func TestUnknownCommand_GuardLeavesRetiredStubsAlone(t *testing.T) {
	bin := buildTestBinary(t)

	_, stderr, code := runRetired(t, bin, "course", "student")
	if code != 4 {
		t.Errorf("exit code = %d, want 4 — the guard must not shadow retired stubs", code)
	}
	if !strings.Contains(stderr, "removed in Andamio CLI 1.0") {
		t.Errorf("retired stub lost its message:\n%s", stderr)
	}
}

// The coverage test. guardUnknownSubcommands originally ran from an init(),
// which walked a half-built tree — Go runs init() in filename order, so every
// command registered by a file sorting after "unknown.go" kept the exit-0
// behavior and nothing failed. This asserts the walk actually reaches every
// group, so moving it back into an init() (or adding a group the walk misses)
// breaks a test rather than silently regressing.
func TestGuardUnknownSubcommands_ReachesEveryGroup(t *testing.T) {
	guardUnknownSubcommands(rootCmd)

	var unguarded []string
	var walk func(cmd *cobra.Command, path string)
	walk = func(cmd *cobra.Command, path string) {
		for _, child := range cmd.Commands() {
			childPath := strings.TrimSpace(path + " " + child.Name())
			walk(child, childPath)
		}
		if len(cmd.Commands()) > 0 && cmd.Run == nil && cmd.RunE == nil {
			unguarded = append(unguarded, path)
		}
	}
	walk(rootCmd, "andamio")

	if len(unguarded) > 0 {
		t.Errorf("these groups would print help and exit 0 on an unknown subcommand: %v", unguarded)
	}
}

// Applying the guard twice must not change behavior — main() calls it once,
// but a test calling it again should not corrupt the tree.
func TestGuardUnknownSubcommands_IsIdempotent(t *testing.T) {
	root := &cobra.Command{Use: "andamio"}
	group := &cobra.Command{Use: "course"}
	group.AddCommand(&cobra.Command{Use: "list", RunE: func(*cobra.Command, []string) error { return nil }})
	root.AddCommand(group)

	guardUnknownSubcommands(root)
	first := group.RunE
	guardUnknownSubcommands(root)

	if first == nil || group.RunE == nil {
		t.Fatal("guard did not install a RunE")
	}

	err := group.RunE(group, []string{"bogus"})
	if err == nil {
		t.Error("guard stopped erroring on an unknown subcommand after a second application")
	}
}
