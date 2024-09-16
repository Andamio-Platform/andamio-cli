package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

var GenerateDocs = &cobra.Command{
	Use:   "generate-docs",
	Short: "Generate Markdown documentation for the CLI",
	Long:  `This command generates Markdown documentation for all commands in the CLI application.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Specify the directory where the docs will be generated
		docsDir := "./docs"

		// Create the directory if it doesn't exist
		if _, err := os.Stat(docsDir); os.IsNotExist(err) {
			if err := os.Mkdir(docsDir, 0755); err != nil {
				return fmt.Errorf("failed to create docs directory: %w", err)
			}
		}

		// Generate the Markdown documentation
		if err := doc.GenMarkdownTree(rootCmd, docsDir); err != nil {
			return fmt.Errorf("failed to generate docs: %w", err)
		}

		fmt.Printf("Documentation successfully generated in %s\n", docsDir)
		return nil
	},
}
