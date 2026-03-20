package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"

	"github.com/Andamio-Platform/andamio-cli/internal/apierr"
	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/Andamio-Platform/andamio-cli/internal/config"
	"github.com/Andamio-Platform/andamio-cli/internal/output"
	"github.com/spf13/cobra"
)

func init() {
	courseCmd.AddCommand(courseImportAllCmd)
	courseImportAllCmd.Flags().String("course-id", "", "Course ID to import into")
	courseImportAllCmd.Flags().String("course", "", "Course name or substring (alternative to --course-id)")
	courseImportAllCmd.Flags().Bool("create", false, "Create modules that don't exist")
	courseImportAllCmd.Flags().Bool("dry-run", false, "Show what would be imported without sending")
	courseImportAllCmd.Flags().Bool("continue-on-error", false, "Continue past failures")
	courseImportAllCmd.Flags().Int("sort-order-start", 0, "Starting sort order for --create (increments per module)")
}

var courseImportAllCmd = &cobra.Command{
	Use:   "import-all <dir>",
	Short: "Import all modules in a compiled course directory",
	Long: `Import all module subdirectories in a compiled course directory.

Scans for subdirectories containing outline.md and imports each one.

Examples:
  andamio course import-all ./compiled/my-course --course-id <id>
  andamio course import-all ./compiled/my-course --course-id <id> --create
  andamio course import-all ./compiled/my-course --course "Intro to Cardano"

Requires user authentication via 'andamio user login'.`,
	Args: cobra.ExactArgs(1),
	PreRunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		if !cfg.HasUserAuth() {
			return &apierr.AuthError{Message: "not authenticated. Run 'andamio user login' first"}
		}
		return nil
	},
	RunE: runImportAll,
}

// ModuleImportSummary holds the result of importing one module
type ModuleImportSummary struct {
	Dir    string        `json:"dir"`
	Code   string        `json:"code"`
	Title  string        `json:"title"`
	Result *ImportResult `json:"result,omitempty"`
	Error  string        `json:"error,omitempty"`
}

func runImportAll(cmd *cobra.Command, args []string) error {
	baseDir := args[0]
	createMode, _ := cmd.Flags().GetBool("create")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	continueOnError, _ := cmd.Flags().GetBool("continue-on-error")
	sortOrderStart, _ := cmd.Flags().GetInt("sort-order-start")
	isJSON := output.GetFormat() == output.FormatJSON

	// Resolve course ID from --course-id or --course flag
	courseID, err := resolveCourseIDFromFlags(cmd)
	if err != nil {
		return err
	}

	// Find module subdirectories (contain outline.md)
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return fmt.Errorf("failed to read directory: %w", err)
	}

	var moduleDirs []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		outlinePath := filepath.Join(baseDir, entry.Name(), "outline.md")
		if _, err := os.Stat(outlinePath); err == nil {
			moduleDirs = append(moduleDirs, entry.Name())
		}
	}

	if len(moduleDirs) == 0 {
		return fmt.Errorf("no module directories found in %s (looking for subdirectories with outline.md)", baseDir)
	}

	// Sort numerically by directory name
	sort.Slice(moduleDirs, func(i, j int) bool {
		numI, errI := strconv.Atoi(moduleDirs[i])
		numJ, errJ := strconv.Atoi(moduleDirs[j])
		if errI == nil && errJ == nil {
			return numI < numJ
		}
		return moduleDirs[i] < moduleDirs[j]
	})

	if !isJSON {
		fmt.Printf("Found %d module(s) in %s\n\n", len(moduleDirs), baseDir)
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	c := client.New(cfg)

	var summaries []ModuleImportSummary
	var errorCount int

	for i, dirName := range moduleDirs {
		moduleDir := filepath.Join(baseDir, dirName)
		sortOrder := sortOrderStart + i

		summary := importSingleModule(c, cfg, moduleDir, courseID, createMode, dryRun, sortOrder, isJSON)
		summaries = append(summaries, summary)

		if summary.Error != "" {
			errorCount++
			if !continueOnError {
				if isJSON {
					return output.PrintJSON(summaries)
				}
				return fmt.Errorf("stopped at %s: %s", dirName, summary.Error)
			}
		}
	}

	if isJSON {
		return output.PrintJSON(summaries)
	}

	// Print summary table
	fmt.Printf("\nImported %d module(s):\n", len(summaries))
	for _, s := range summaries {
		if s.Error != "" {
			fmt.Printf("  %-6s %-40s  FAILED: %s\n", s.Code, s.Title, s.Error)
		} else if s.Result != nil {
			changes := ""
			if v, ok := s.Result.Changes["lessons_created"].(float64); ok && v > 0 {
				changes += fmt.Sprintf("%.0f created", v)
			}
			if v, ok := s.Result.Changes["lessons_updated"].(float64); ok && v > 0 {
				if changes != "" {
					changes += ", "
				}
				changes += fmt.Sprintf("%.0f updated", v)
			}
			if changes == "" {
				changes = "no changes"
			}
			fmt.Printf("  %-6s %-40s  %d SLTs, %s\n", s.Code, s.Title, s.Result.SLTCount, changes)
		}
	}

	if errorCount > 0 {
		fmt.Printf("\n%d module(s) failed\n", errorCount)
	}

	return nil
}

func importSingleModule(c *client.Client, cfg *config.Config, moduleDir, courseID string, createMode, dryRun bool, sortOrder int, isJSON bool) ModuleImportSummary {
	summary := ModuleImportSummary{Dir: filepath.Base(moduleDir)}

	if !isJSON {
		fmt.Printf("Importing %s...\n", filepath.Base(moduleDir))
	}

	// Read outline metadata for summary (lightweight — no lesson parsing)
	title, code, err := readOutlineMetadata(moduleDir)
	if err != nil {
		summary.Error = err.Error()
		return summary
	}
	summary.Code = code
	summary.Title = title

	// Delegate to shared import orchestration
	result, err := importModule(ImportParams{
		Client:     c,
		Config:     cfg,
		ModuleDir:  moduleDir,
		CourseID:   courseID,
		CreateMode: createMode,
		DryRun:     dryRun,
		SortOrder:  sortOrder,
		Quiet:      isJSON,
	})
	if err != nil {
		summary.Error = err.Error()
		return summary
	}

	summary.Result = result
	return summary
}
