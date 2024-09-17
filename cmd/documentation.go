package cmd

import (
	"fmt"
	"log"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

// gendocCmd represents the Cobra command to generate the documentation in Markdown format
var docCmd = &cobra.Command{
	Use:   "docs",
	Short: "Generate documentation in Markdown format organized by subcommands",
	Long:  "Generate Markdown documentation for all commands, organized in subfolders based on subcommands.",
	RunE: func(cmd *cobra.Command, args []string) error {
		docsDir := "./docs"

		err := doc.GenMarkdownTree(rootCmd, docsDir)
		if err != nil {
			log.Fatal(err)
		}

		fmt.Printf("Documentation successfully generated in %s\n", docsDir)
		return nil
	},
}
