package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Andamio-Platform/andamio-cli/internal/schemasnapshot"
	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

func Compare(cmd *cobra.Command, args []string) error {

	tempDir, err := os.MkdirTemp("", "temp")
	if err != nil {
		return fmt.Errorf("unable to create temp directory %w", err)
	}

	rootCmd.DisableAutoGenTag = true
	err = doc.GenMarkdownTree(rootCmd, tempDir)
	if err != nil {
		return fmt.Errorf("could not generate markdown tree, %w", err)
	}

	err = schemasnapshot.Generate("./cmd/andamio", filepath.Join(tempDir, "/schema.txt"))
	if err != nil {
		return fmt.Errorf("could not generate schema snapshot @ %s, %w ", tempDir, err)
	}

	return nil
}

var compareCmd = &cobra.Command{
	Hidden: true,
	Short:  "compare a new snapshot against the baseline snapshot already commited ",
	Use:    "compare",
	RunE:   Compare,
}

func init() {
	rootCmd.AddCommand(compareCmd)
}

func diffDirs(baselineDir, tempDir string) ([]string, error) {

	baselineDirectories, err := os.ReadDir(baselineDir)
	if err != nil {
		return nil, fmt.Errorf("could not read directory %s, %w", baselineDir, err)
	}

	tempDirectories, err := os.ReadDir(tempDir)
	if err != nil {
		return nil, fmt.Errorf("could not read directory %s, %w", tempDir, err)
	}

	var differences []string
	baselineFiles := make(map[string]bool)
	tempFiles := make(map[string]bool)

	for _, b := range baselineDirectories {
		baselineFiles[b.Name()] = true
	}

	for _, t := range tempDirectories {
		tempFiles[t.Name()] = true
	}

	for name := range baselineFiles {
		if !tempFiles[name] {
			differences = append(differences, "removed "+name)
		}
	}

	for name := range tempFiles {
		if !baselineFiles[name] {
			differences = append(differences, "added "+name)
		}
	}

	return differences, nil

}
