package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

// generateMarkdown generates documentation and organizes it in subfolders by command hierarchy.
func generateMarkdown(cmd *cobra.Command, dir string) error {
	// Make the directory if it doesn't exist
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}
	}

	// Create the file for the command markdown
	filename := filepath.Join(dir, cmd.Name()+".md")
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create markdown file for command %s: %w", cmd.Name(), err)
	}
	defer file.Close()

	// Generate the markdown for this command and write to the file
	err = doc.GenMarkdownCustom(cmd, file, linkHandler)
	if err != nil {
		return fmt.Errorf("failed to generate markdown for command %s: %w", cmd.Name(), err)
	}

	// Recursively generate markdown for each subcommand in its own subfolder
	for _, subCmd := range cmd.Commands() {
		subDir := filepath.Join(dir, cmd.Name()) // Create a folder named after the command
		if err := generateMarkdown(subCmd, subDir); err != nil {
			return err
		}
	}

	return nil
}

// filePrepender adds a front matter header (optional, for static site generators like Hugo)
func filePrepender(filename string) string {
	return "---\ntitle: \"" + strings.TrimSuffix(filename, ".md") + "\"\n---\n"
}

// linkHandler handles link generation between commands
func linkHandler(name string) string {
	return fmt.Sprintf("%s.md", strings.ToLower(name))
}

var DocCmd = &cobra.Command{
	Use:   "doc",
	Short: "Generate andamio-cli documentation",
	Long: `
Generate markdown docs in ./docs	
	`,

	RunE: func(cmd *cobra.Command, args []string) error {
		docsDir := "./docs"

		// Start generating documentation from the root command
		err := generateMarkdown(rootCmd, docsDir)
		if err != nil {
			return fmt.Errorf("failed to generate documentation: %w", err)
		}

		fmt.Printf("Documentation successfully generated in %s\n", docsDir)
		return nil
	},
}
