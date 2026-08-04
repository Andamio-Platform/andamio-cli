package main

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

// checking CHANGELOG.md was touched vs the base branch

func changelogTouched(baseBranch string) (bool, error) {

	gitCmd := exec.Command("git", "diff", "--name-only", baseBranch+"...HEAD")
	output, err := gitCmd.Output()
	if err != nil {
		return false, fmt.Errorf("could not run git diff against %s, %w", baseBranch, err)
	}

	changedFiles := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, f := range changedFiles {
		if f == "CHANGELOG.md" {
			return true, nil
		}
	}

	return false, nil
}

func ChangelogCheck(cmd *cobra.Command, args []string) error {

	touched, err := changelogTouched("origin/main")
	if err != nil {
		return fmt.Errorf("could not check changelog, %w", err)
	}

	if !touched {
		return fmt.Errorf("CHANGELOG.md was not updated on this branch")
	}

	fmt.Println("CHANGELOG.md was updated")
	return nil
}

var changelogCmd = &cobra.Command{
	Hidden: true,
	Short:  "check that CHANGELOG.md was updated against the base branch",
	Use:    "changelog-check",
	RunE:   ChangelogCheck,
}

func init() {
	rootCmd.AddCommand(changelogCmd)
}
