package cmd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

func GenMarkdownTreeCustom(cmd *cobra.Command, dir string) error {
	for _, c := range cmd.Commands() {
		if !c.IsAvailableCommand() || c.IsAdditionalHelpTopicCommand() {
			continue
		}

		// Create subfolder path
		// Fix: Use strings.Split to convert CommandPath to a slice
		cmdPath := strings.Split(c.CommandPath(), " ")
		subDir := filepath.Join(dir, filepath.Join(cmdPath[1:]...))
		if err := os.MkdirAll(subDir, 0755); err != nil {
			return err
		}

		// Generate markdown file
		// Fix: Use a bytes.Buffer to capture the output
		var buf bytes.Buffer
		if err := doc.GenMarkdown(c, &buf); err != nil {
			return err
		}

		// Write to file
		filename := filepath.Join(subDir, c.Name()+".md")
		if err := os.WriteFile(filename, buf.Bytes(), 0644); err != nil {
			return err
		}

		// Recursively generate for subcommands
		if err := GenMarkdownTreeCustom(c, dir); err != nil {
			return err
		}
	}

	return nil
}

var docCmd = &cobra.Command{
	Use:   "docs",
	Short: "Generate Andamio CLI documentation",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		err := GenMarkdownTreeCustom(rootCmd, "./docs")
		if err != nil {
			fmt.Printf("Error generating documentation: %s\n", err)
			os.Exit(1)

		}
	},
}
