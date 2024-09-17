package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

// generateMarkdown generates documentation and organizes it in subfolders by command hierarchy.
func generateMarkdown(cmd *cobra.Command, dir string) error {
	// Ensure the directory exists
	if err := ensureDir(dir); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	// Sanitize command name for file path and add ".md" for the actual file name
	filename := filepath.Join(dir, sanitizeFileName(cmd.Name())+".md")
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create markdown file for command %s: %w", cmd.Name(), err)
	}
	defer file.Close()

	// Generate the markdown for this command and write to the file
	// We now explicitly pass a simple empty link handler
	if err := doc.GenMarkdownCustom(cmd, file, func(name string) string {
		// This is an empty handler that simply returns the name itself (you can modify this if needed)
		return name
	}); err != nil {
		return fmt.Errorf("failed to generate markdown for command %s: %w", cmd.Name(), err)
	}

	// Recursively generate markdown for each subcommand in its own subfolder
	for _, subCmd := range cmd.Commands() {
		if !subCmd.Hidden {
			subDir := filepath.Join(dir, sanitizeFileName(cmd.Name())) // Create a folder named after the command
			if err := generateMarkdown(subCmd, subDir); err != nil {
				return err
			}
		}
	}

	return nil
}

// ensureDir ensures that a directory exists, creating it if necessary.
func ensureDir(dir string) error {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("could not create directory: %w", err)
		}
	}
	return nil
}

// sanitizeFileName ensures that file and folder names are safe by removing or replacing invalid characters
// while preserving dots for file extensions.
func sanitizeFileName(name string) string {
	// Replace spaces and special characters (except ".") with a hyphen
	reg := regexp.MustCompile(`[^a-zA-Z0-9_.-]+`)
	return reg.ReplaceAllString(strings.ToLower(name), "-")
}

// filePrepender adds a front matter header (optional, for static site generators like Hugo)
func filePrepender(filename string) string {
	return fmt.Sprintf("---\ntitle: \"%s\"\n---\n", strings.TrimSuffix(filepath.Base(filename), ".md"))
}

// gendocCmd represents the Cobra command to generate the documentation in Markdown format
var docCmd = &cobra.Command{
	Use:   "docs",
	Short: "Generate documentation in Markdown format organized by subcommands",
	Long:  "Generate Markdown documentation for all commands, organized in subfolders based on subcommands.",
	RunE: func(cmd *cobra.Command, args []string) error {
		docsDir := "./docs"

		// Start generating documentation from the root command
		if err := generateMarkdown(rootCmd, docsDir); err != nil {
			return fmt.Errorf("failed to generate documentation: %w", err)
		}

		fmt.Printf("Documentation successfully generated in %s\n", docsDir)
		return nil
	},
}

func init() {
	// Register the gendocCmd as part of the root command
	rootCmd.AddCommand(docCmd)
}
