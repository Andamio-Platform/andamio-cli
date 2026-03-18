package main

import (
	"fmt"
	"os"

	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/Andamio-Platform/andamio-cli/internal/config"
	"github.com/Andamio-Platform/andamio-cli/internal/output"
	"github.com/spf13/cobra"
)

func init() {
	courseCmd.AddCommand(courseCreateModuleCmd)
	courseCreateModuleCmd.Flags().String("course-id", "", "Course ID (required)")
	courseCreateModuleCmd.MarkFlagRequired("course-id")
	courseCreateModuleCmd.Flags().String("code", "", "Module code (reads from outline.md if path provided)")
	courseCreateModuleCmd.Flags().String("title", "", "Module title (reads from outline.md if path provided)")
	courseCreateModuleCmd.Flags().Int("sort-order", 0, "Sort order for the module (default: 0)")
}

var courseCreateModuleCmd = &cobra.Command{
	Use:   "create-module [path]",
	Short: "Create a new course module",
	Long: `Create a new course module, optionally reading metadata from a compiled directory.

With a path argument, reads title and code from outline.md:
  andamio course create-module ./compiled/my-course/101 --course-id <id>

With explicit flags (no path needed):
  andamio course create-module --course-id <id> --code 101 --title "My Module"

Requires user authentication via 'andamio user login'.`,
	Args: cobra.MaximumNArgs(1),
	PreRunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		if !cfg.HasUserAuth() {
			return fmt.Errorf("not authenticated. Run 'andamio user login' first")
		}
		return nil
	},
	RunE: runCreateModule,
}

// CreateModuleResult holds the result for structured output
type CreateModuleResult struct {
	CourseID   string `json:"course_id"`
	ModuleCode string `json:"module_code"`
	Title      string `json:"title"`
	SortOrder  int    `json:"sort_order"`
}

func runCreateModule(cmd *cobra.Command, args []string) error {
	courseID, _ := cmd.Flags().GetString("course-id")
	code, _ := cmd.Flags().GetString("code")
	title, _ := cmd.Flags().GetString("title")
	sortOrder, _ := cmd.Flags().GetInt("sort-order")
	isJSON := output.GetFormat() == output.FormatJSON

	// If a path is provided, read outline.md for metadata (lightweight — no lesson parsing)
	if len(args) > 0 {
		outlineTitle, outlineCode, err := readOutlineMetadata(args[0])
		if err != nil {
			return err
		}
		if code == "" {
			code = outlineCode
		}
		if title == "" {
			title = outlineTitle
		}
	}

	if code == "" {
		return fmt.Errorf("module code required: provide --code flag or a path with outline.md")
	}
	if title == "" {
		return fmt.Errorf("module title required: provide --title flag or a path with outline.md")
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	c := client.New(cfg)

	if !isJSON {
		fmt.Fprintf(os.Stderr, "Creating module %s (%s)...\n", title, code)
	}

	// Step 1: Create the module shell
	createPayload := map[string]interface{}{
		"course_id":          courseID,
		"course_module_code": code,
		"title":              title,
		"sort_order":         sortOrder,
	}

	var createResp map[string]interface{}
	if err := c.Post("/api/v2/course/teacher/course-module/create", createPayload, &createResp); err != nil {
		return fmt.Errorf("failed to create module: %w", err)
	}

	result := CreateModuleResult{
		CourseID:   courseID,
		ModuleCode: code,
		Title:      title,
		SortOrder:  sortOrder,
	}

	if isJSON {
		return output.PrintJSON(result)
	}

	fmt.Fprintf(os.Stderr, "Created module: %s (%s)\n", title, code)
	return nil
}
