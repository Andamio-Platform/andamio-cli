package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"

	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/Andamio-Platform/andamio-cli/internal/config"
	"github.com/Andamio-Platform/andamio-cli/internal/output"
	"github.com/spf13/cobra"
)

func init() {
	courseCmd.AddCommand(courseImportAllCmd)
	courseImportAllCmd.Flags().String("course-id", "", "Course ID to import into (required)")
	courseImportAllCmd.MarkFlagRequired("course-id")
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

Example:
  andamio course import-all ./compiled/my-course --course-id <id>
  andamio course import-all ./compiled/my-course --course-id <id> --create

Requires user authentication via 'andamio user login'.`,
	Args: cobra.ExactArgs(1),
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
	RunE: runImportAll,
}

// ModuleImportSummary holds the result of importing one module
type ModuleImportSummary struct {
	Dir        string        `json:"dir"`
	Code       string        `json:"code"`
	Title      string        `json:"title"`
	Result     *ImportResult `json:"result,omitempty"`
	Error      string        `json:"error,omitempty"`
	Skipped    bool          `json:"skipped,omitempty"`
}

func runImportAll(cmd *cobra.Command, args []string) error {
	baseDir := args[0]
	courseID, _ := cmd.Flags().GetString("course-id")
	createMode, _ := cmd.Flags().GetBool("create")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	continueOnError, _ := cmd.Flags().GetBool("continue-on-error")
	sortOrderStart, _ := cmd.Flags().GetInt("sort-order-start")
	isJSON := output.GetFormat() == output.FormatJSON

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

	// Read and parse
	data, err := readCompiledModule(moduleDir)
	if err != nil {
		summary.Error = err.Error()
		return summary
	}
	summary.Code = data.ModuleCode
	summary.Title = data.Title

	// Upload new images
	var imagesUploaded int
	if len(data.ImageWarnings) > 0 {
		assetsDir := filepath.Join(moduleDir, "assets")
		var failed []string
		imagesUploaded, failed = uploadNewImages(cfg, assetsDir, data.ImageWarnings, data.ImageManifest)
		if imagesUploaded > 0 {
			manifestData, _ := json.MarshalIndent(data.ImageManifest, "", "  ")
			os.WriteFile(filepath.Join(assetsDir, ".image-manifest.json"), manifestData, 0644)
			data, err = readCompiledModule(moduleDir)
			if err != nil {
				summary.Error = err.Error()
				return summary
			}
		}
		data.ImageWarnings = failed
	}

	// Fetch existing or create
	existing, err := fetchExistingModule(c, courseID, data.ModuleCode)
	if err != nil {
		if createMode && errors.Is(err, errModuleNotFound) {
			if !isJSON {
				fmt.Printf("  Module %s not found — creating...\n", data.ModuleCode)
			}
			createPayload := map[string]interface{}{
				"course_id":          courseID,
				"course_module_code": data.ModuleCode,
				"title":              data.Title,
				"sort_order":         sortOrder,
			}
			var createResp map[string]interface{}
			if err := c.Post("/api/v2/course/teacher/course-module/create", createPayload, &createResp); err != nil {
				summary.Error = fmt.Sprintf("failed to create module: %v", err)
				return summary
			}
			existing, err = fetchExistingModule(c, courseID, data.ModuleCode)
			if err != nil {
				summary.Error = fmt.Sprintf("failed to fetch created module: %v", err)
				return summary
			}
		} else {
			summary.Error = err.Error()
			return summary
		}
	}

	sltsLocked := existing.Status != "DRAFT"

	// Update
	resp, err := updateModuleContent(c, courseID, data, existing, sltsLocked, dryRun)
	if err != nil {
		summary.Error = err.Error()
		return summary
	}

	changes, _ := resp["changes"].(map[string]interface{})
	if changes == nil {
		changes = map[string]interface{}{}
	}

	summary.Result = &ImportResult{
		CourseID:       courseID,
		ModuleCode:     data.ModuleCode,
		Title:          data.Title,
		ModuleStatus:   existing.Status,
		SLTsLocked:     sltsLocked,
		SLTCount:       len(data.SLTs),
		LessonCount:    len(data.Lessons),
		HasIntro:       data.Introduction != nil,
		HasAssignment:  data.Assignment != nil,
		ManifestUsed:   len(data.ImageManifest),
		ImagesUploaded: imagesUploaded,
		DryRun:         dryRun,
		Changes:        changes,
	}

	return summary
}
