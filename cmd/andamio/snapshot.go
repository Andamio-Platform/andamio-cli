package main

import (
	"fmt"
	"os"

	"github.com/Andamio-Platform/andamio-cli/internal/schemasnapshot"
	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

// creating command/flag snapshot

func callSnapshot(cmd *cobra.Command, args []string) error {

	if err := os.RemoveAll("./cobra-snapshots"); err != nil {
		return fmt.Errorf("could not clear existing snapshot directory: %w", err)
	}
	os.MkdirAll("./cobra-snapshots", 0755)
	rootCmd.DisableAutoGenTag = true
	err := doc.GenMarkdownTree(rootCmd, "./cobra-snapshots")
	if err != nil {
		return fmt.Errorf("Could not generate markdown file , %s", err)
	}

	if err := schemasnapshot.Generate("./cmd/andamio", "./cobra-snapshots/schema.txt"); err != nil {
		return fmt.Errorf("could not generate schema snapshot: %w", err)
	}

	return nil
}

var snapshotCmd = &cobra.Command{
	Hidden: true,
	Short:  "Pull snaphot of current cmd output and json structs",
	Use:    "snapshot",
	RunE:   callSnapshot,
}

func init() {
	rootCmd.AddCommand(snapshotCmd)
}
