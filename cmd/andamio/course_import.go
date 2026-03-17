package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/Andamio-Platform/andamio-cli/internal/config"
	"github.com/Andamio-Platform/andamio-cli/internal/output"
	"github.com/adrg/frontmatter"
	"github.com/spf13/cobra"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

var (
	sltLineRe          = regexp.MustCompile(`^\d+[\.\)]\s+(.+)$`)
	lessonNumRe        = regexp.MustCompile(`lesson-(\d+)\.md`)
	errModuleNotFound  = errors.New("module not found")
)

func init() {
	courseCmd.AddCommand(courseImportCmd)
	courseImportCmd.Flags().String("course-id", "", "Course ID to import into (required)")
	courseImportCmd.MarkFlagRequired("course-id")
	courseImportCmd.Flags().Bool("dry-run", false, "Show the API payload without sending it")
	courseImportCmd.Flags().Bool("create", false, "Create the module if it doesn't exist")
	courseImportCmd.Flags().Int("sort-order", 0, "Sort order when creating a new module (used with --create)")
}

var courseImportCmd = &cobra.Command{
	Use:   "import <path>",
	Short: "Import a compiled module to update course content",
	Long: `Import a compiled module directory to update an existing course module.

The directory should contain:
  - outline.md (with YAML frontmatter: title, code)
  - lesson-N.md files (one per SLT)
  - introduction.md (optional)
  - assignment.md (optional)

Example:
  andamio course import ./compiled/my-course/101 --course-id abc123

New images in assets/ are automatically uploaded to the CDN.
Previously uploaded images are preserved via .image-manifest.json.

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
	RunE: runCourseImport,
}

// ImportData holds the parsed module content for import
type ImportData struct {
	Title         string
	ModuleCode    string
	SLTs          []string
	Lessons       []LessonImport
	Introduction  *ContentSection
	Assignment    *ContentSection
	ImageWarnings []string
	ImageManifest map[string]string // filename → original URL from .image-manifest.json
}

// LessonImport holds a single lesson's data
type LessonImport struct {
	Index      int
	Title      string
	TiptapJSON map[string]interface{}
}

// ContentSection holds parsed content with title extracted from H1
type ContentSection struct {
	Title      string
	TiptapJSON map[string]interface{}
}

// OutlineFrontmatter is the YAML frontmatter structure in outline.md
type OutlineFrontmatter struct {
	Title string `yaml:"title"`
	Code  string `yaml:"code"`
}

// ImportResult holds the result of an import operation for structured output
type ImportResult struct {
	CourseID       string                 `json:"course_id"`
	ModuleCode     string                 `json:"module_code"`
	Title          string                 `json:"title"`
	ModuleStatus   string                 `json:"module_status"`
	SLTsLocked     bool                   `json:"slts_locked"`
	SLTCount       int                    `json:"slt_count"`
	LessonCount    int                    `json:"lesson_count"`
	HasIntro       bool                   `json:"has_introduction"`
	HasAssignment  bool                   `json:"has_assignment"`
	ManifestUsed   int                    `json:"manifest_images"`
	ImagesUploaded int                    `json:"images_uploaded,omitempty"`
	FailedImages   []string               `json:"failed_images,omitempty"`
	DryRun         bool                   `json:"dry_run,omitempty"`
	Changes        map[string]interface{} `json:"changes"`
}

// ImportParams holds the parameters for importing a single module.
type ImportParams struct {
	Client     *client.Client
	Config     *config.Config
	ModuleDir  string
	CourseID   string
	CreateMode bool
	DryRun     bool
	SortOrder  int
	Quiet      bool // suppress progress output (JSON mode)
}

// importModule is the shared orchestration logic for importing a single module.
// Used by both `course import` and `course import-all`.
func importModule(p ImportParams) (*ImportResult, error) {
	// Read and parse the compiled module
	data, err := readCompiledModule(p.ModuleDir)
	if err != nil {
		return nil, err
	}

	if !p.Quiet && len(data.ImageManifest) > 0 {
		fmt.Printf("Found image manifest: %d image(s) will use original URLs\n", len(data.ImageManifest))
	}

	// Upload new images (not in manifest) to the app's CDN, then update manifest on disk
	var imagesUploaded int
	if len(data.ImageWarnings) > 0 {
		if p.DryRun {
			if !p.Quiet {
				fmt.Printf("Dry-run: %d new image(s) would be uploaded:\n", len(data.ImageWarnings))
				for _, img := range data.ImageWarnings {
					fmt.Printf("  %s\n", img)
				}
			}
		} else {
			if !p.Quiet {
				fmt.Printf("Uploading %d new image(s)...\n", len(data.ImageWarnings))
			}
			assetsDir := filepath.Join(p.ModuleDir, "assets")
			var failed []string
			imagesUploaded, failed = uploadNewImages(p.Config, assetsDir, data.ImageWarnings, data.ImageManifest)

			if !p.Quiet {
				if imagesUploaded > 0 {
					fmt.Printf("  Uploaded %d image(s)\n", imagesUploaded)
				}
				for _, f := range failed {
					fmt.Printf("  Failed: %s\n", f)
				}
			}

			// Write updated manifest to disk so re-read picks up new URLs
			if imagesUploaded > 0 {
				manifestData, _ := json.MarshalIndent(data.ImageManifest, "", "  ")
				os.WriteFile(filepath.Join(assetsDir, ".image-manifest.json"), manifestData, 0644)

				// Re-read module with updated manifest (new URLs now resolve during conversion)
				data, err = readCompiledModule(p.ModuleDir)
				if err != nil {
					return nil, err
				}
			}

			data.ImageWarnings = failed
		}
	}

	// Fetch current module state to determine SLT lock status and preserve metadata
	existing, err := fetchExistingModule(p.Client, p.CourseID, data.ModuleCode)
	if err != nil {
		// Only trigger creation for "not found" errors, not auth/network failures
		if p.CreateMode && errors.Is(err, errModuleNotFound) {
			if p.DryRun {
				if !p.Quiet {
					fmt.Printf("Dry-run: would create module %s (%s) with sort_order %d\n", data.Title, data.ModuleCode, p.SortOrder)
				}
				return &ImportResult{
					CourseID:   p.CourseID,
					ModuleCode: data.ModuleCode,
					Title:      data.Title,
					DryRun:     true,
					SLTCount:   len(data.SLTs),
					LessonCount: len(data.Lessons),
					HasIntro:   data.Introduction != nil,
					HasAssignment: data.Assignment != nil,
					Changes:    map[string]interface{}{"would_create_module": true},
				}, nil
			}
			if !p.Quiet {
				fmt.Printf("Module %s not found — creating...\n", data.ModuleCode)
			}
			createPayload := map[string]interface{}{
				"course_id":          p.CourseID,
				"course_module_code": data.ModuleCode,
				"title":              data.Title,
				"sort_order":         p.SortOrder,
			}
			var createResp map[string]interface{}
			if err := p.Client.Post("/api/v2/course/teacher/course-module/create", createPayload, &createResp); err != nil {
				return nil, fmt.Errorf("failed to create module: %w", err)
			}
			if !p.Quiet {
				fmt.Printf("Created module: %s (%s)\n", data.Title, data.ModuleCode)
			}
			// Re-fetch the newly created module
			existing, err = fetchExistingModule(p.Client, p.CourseID, data.ModuleCode)
			if err != nil {
				return nil, fmt.Errorf("failed to fetch newly created module: %w", err)
			}
		} else {
			return nil, err
		}
	}
	sltsLocked := existing.Status != "DRAFT"
	if !p.Quiet && sltsLocked {
		fmt.Printf("Module status is %s — SLTs are locked, updating content only\n", existing.Status)
	}

	// Update the module via API (or dump payload in dry-run mode)
	resp, err := updateModuleContent(p.Client, p.CourseID, data, existing, sltsLocked, p.DryRun)
	if err != nil {
		return nil, err
	}

	// Extract changes summary from API response
	changes, _ := resp["changes"].(map[string]interface{})
	if changes == nil {
		changes = map[string]interface{}{}
	}

	return &ImportResult{
		CourseID:       p.CourseID,
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
		FailedImages:   data.ImageWarnings,
		DryRun:         p.DryRun,
		Changes:        changes,
	}, nil
}

func runCourseImport(cmd *cobra.Command, args []string) error {
	moduleDir := args[0]
	courseID, _ := cmd.Flags().GetString("course-id")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	createMode, _ := cmd.Flags().GetBool("create")
	sortOrder, _ := cmd.Flags().GetInt("sort-order")
	isJSON := output.GetFormat() == output.FormatJSON

	// Validate directory exists
	info, err := os.Stat(moduleDir)
	if err != nil {
		return fmt.Errorf("directory not found: %s", moduleDir)
	}
	if !info.IsDir() {
		return fmt.Errorf("not a directory: %s", moduleDir)
	}

	if !isJSON {
		fmt.Printf("Reading module from %s...\n", moduleDir)
	}

	// Load config and create client
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	c := client.New(cfg)

	importResult, err := importModule(ImportParams{
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
		return err
	}

	if isJSON {
		return output.PrintJSON(importResult)
	}

	// Text TUI output
	r := importResult
	fmt.Println()
	fmt.Printf("  Module:  %s (%s)\n", r.Title, r.ModuleCode)
	fmt.Printf("  Course:  %s\n", r.CourseID)
	fmt.Println()
	fmt.Println("  Content:")
	fmt.Printf("    SLTs:          %d\n", r.SLTCount)
	fmt.Printf("    Lessons:       %d\n", r.LessonCount)
	if r.HasIntro {
		fmt.Printf("    Introduction:  yes\n")
	}
	if r.HasAssignment {
		fmt.Printf("    Assignment:    yes\n")
	}

	// Show changes from API response
	changes := r.Changes
	if len(changes) > 0 {
		fmt.Println()
		fmt.Println("  Changes:")
		if v, ok := changes["slts_created"].(float64); ok && v > 0 {
			fmt.Printf("    SLTs created:       %.0f\n", v)
		}
		if v, ok := changes["slts_updated"].(float64); ok && v > 0 {
			fmt.Printf("    SLTs updated:       %.0f\n", v)
		}
		if v, ok := changes["slts_deleted"].(float64); ok && v > 0 {
			fmt.Printf("    SLTs deleted:       %.0f\n", v)
		}
		if v, ok := changes["lessons_created"].(float64); ok && v > 0 {
			fmt.Printf("    Lessons created:    %.0f\n", v)
		}
		if v, ok := changes["lessons_updated"].(float64); ok && v > 0 {
			fmt.Printf("    Lessons updated:    %.0f\n", v)
		}
		if v, ok := changes["lessons_deleted"].(float64); ok && v > 0 {
			fmt.Printf("    Lessons deleted:    %.0f\n", v)
		}
		if v, ok := changes["introduction_created"].(bool); ok && v {
			fmt.Printf("    Introduction:       created\n")
		}
		if v, ok := changes["introduction_updated"].(bool); ok && v {
			fmt.Printf("    Introduction:       updated\n")
		}
		if v, ok := changes["assignment_created"].(bool); ok && v {
			fmt.Printf("    Assignment:         created\n")
		}
		if v, ok := changes["assignment_updated"].(bool); ok && v {
			fmt.Printf("    Assignment:         updated\n")
		}
	}

	if r.ManifestUsed > 0 || r.ImagesUploaded > 0 {
		fmt.Printf("\n  Images:\n")
		if r.ManifestUsed > 0 {
			fmt.Printf("    Manifest:   %d preserved via CDN URLs\n", r.ManifestUsed)
		}
		if r.ImagesUploaded > 0 {
			fmt.Printf("    Uploaded:   %d new image(s) to CDN\n", r.ImagesUploaded)
		}
		if len(r.FailedImages) > 0 {
			fmt.Printf("    Failed:     %d image(s) could not be uploaded\n", len(r.FailedImages))
		}
	}

	fmt.Println()
	return nil
}

// readOutlineMetadata reads only the title and code from outline.md frontmatter.
// Use this when you don't need lesson content (e.g., create-module command).
func readOutlineMetadata(dir string) (title, code string, err error) {
	outlineBytes, err := os.ReadFile(filepath.Join(dir, "outline.md"))
	if err != nil {
		return "", "", fmt.Errorf("missing outline.md: %w", err)
	}
	var fm OutlineFrontmatter
	if _, err := frontmatter.Parse(bytes.NewReader(outlineBytes), &fm); err != nil {
		return "", "", fmt.Errorf("invalid outline.md frontmatter: %w", err)
	}
	if fm.Code == "" {
		return "", "", fmt.Errorf("outline.md missing 'code' in frontmatter")
	}
	return fm.Title, fm.Code, nil
}

func readCompiledModule(dir string) (*ImportData, error) {
	data := &ImportData{}

	// Read outline.md
	outlinePath := filepath.Join(dir, "outline.md")
	outlineBytes, err := os.ReadFile(outlinePath)
	if err != nil {
		return nil, fmt.Errorf("missing outline.md: %w", err)
	}

	var fm OutlineFrontmatter
	content, err := frontmatter.Parse(bytes.NewReader(outlineBytes), &fm)
	if err != nil {
		return nil, fmt.Errorf("invalid outline.md frontmatter: %w", err)
	}

	data.Title = fm.Title
	data.ModuleCode = fm.Code

	if data.ModuleCode == "" {
		return nil, fmt.Errorf("outline.md missing 'code' in frontmatter")
	}

	// Parse SLTs from outline content
	data.SLTs = parseSLTsFromOutline(string(content))

	// Load image manifest BEFORE converting any content
	assetsDir := filepath.Join(dir, "assets")
	data.ImageManifest = loadImageManifest(assetsDir)

	// Check for images in assets/ that are NOT in the manifest (new images)
	if info, err := os.Stat(assetsDir); err == nil && info.IsDir() {
		files, _ := os.ReadDir(assetsDir)
		for _, f := range files {
			if f.IsDir() || f.Name() == ".image-manifest.json" {
				continue
			}
			if _, inManifest := data.ImageManifest[f.Name()]; !inManifest {
				data.ImageWarnings = append(data.ImageWarnings, f.Name())
			}
		}
	}

	// Read lesson files
	lessonFiles, err := filepath.Glob(filepath.Join(dir, "lesson-*.md"))
	if err != nil {
		return nil, fmt.Errorf("failed to find lesson files: %w", err)
	}

	// Sort lesson files by number
	sort.Slice(lessonFiles, func(i, j int) bool {
		numI := extractLessonNumber(lessonFiles[i])
		numJ := extractLessonNumber(lessonFiles[j])
		return numI < numJ
	})

	for _, lessonFile := range lessonFiles {
		lessonNum := extractLessonNumber(lessonFile)
		content, err := os.ReadFile(lessonFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read %s: %w", filepath.Base(lessonFile), err)
		}

		// Extract H1 as title, convert remaining body to Tiptap (matches app behavior)
		title, body := extractH1Title(string(content))
		if title == "" && output.GetFormat() != output.FormatJSON {
			fmt.Printf("Warning: %s has no # title heading — lesson will import without a title\n", filepath.Base(lessonFile))
		}
		tiptap, err := markdownToTiptap(body, data.ImageManifest)
		if err != nil {
			return nil, fmt.Errorf("failed to convert %s: %w", filepath.Base(lessonFile), err)
		}

		data.Lessons = append(data.Lessons, LessonImport{
			Index:      lessonNum,
			Title:      title,
			TiptapJSON: tiptap,
		})
	}

	// Warn if lesson count doesn't match SLT count (lessons are optional per format guide)
	if len(data.Lessons) > len(data.SLTs) {
		return nil, fmt.Errorf("found %d lesson files but outline only lists %d SLTs", len(data.Lessons), len(data.SLTs))
	}
	if len(data.Lessons) < len(data.SLTs) && len(data.Lessons) > 0 {
		if output.GetFormat() != output.FormatJSON {
			fmt.Printf("Note: %d lesson files for %d SLTs (lessons are optional)\n", len(data.Lessons), len(data.SLTs))
		}
	}

	// Read introduction.md if exists — H1 → title, rest → content_json
	introPath := filepath.Join(dir, "introduction.md")
	if introBytes, err := os.ReadFile(introPath); err == nil && len(introBytes) > 0 {
		title, body := extractH1Title(string(introBytes))
		if title == "" && output.GetFormat() != output.FormatJSON {
			fmt.Printf("Warning: introduction.md has no # title heading\n")
		}
		tiptap, err := markdownToTiptap(body, data.ImageManifest)
		if err != nil {
			return nil, fmt.Errorf("failed to convert introduction.md: %w", err)
		}
		data.Introduction = &ContentSection{Title: title, TiptapJSON: tiptap}
	}

	// Read assignment.md if exists — H1 → title, rest → content_json
	assignPath := filepath.Join(dir, "assignment.md")
	if assignBytes, err := os.ReadFile(assignPath); err == nil && len(assignBytes) > 0 {
		title, body := extractH1Title(string(assignBytes))
		if title == "" && output.GetFormat() != output.FormatJSON {
			fmt.Printf("Warning: assignment.md has no # title heading\n")
		}
		tiptap, err := markdownToTiptap(body, data.ImageManifest)
		if err != nil {
			return nil, fmt.Errorf("failed to convert assignment.md: %w", err)
		}
		data.Assignment = &ContentSection{Title: title, TiptapJSON: tiptap}
	}

	return data, nil
}

// loadImageManifest reads .image-manifest.json from the assets directory.
// Returns an empty map if the file doesn't exist.
// Warns on parse errors rather than silently degrading.
func loadImageManifest(assetsDir string) map[string]string {
	manifestPath := filepath.Join(assetsDir, ".image-manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		if !os.IsNotExist(err) && output.GetFormat() != output.FormatJSON {
			fmt.Printf("Warning: could not read image manifest: %v\n", err)
		}
		return make(map[string]string)
	}

	var raw map[string]string
	if err := json.Unmarshal(data, &raw); err != nil {
		if output.GetFormat() != output.FormatJSON {
			fmt.Printf("Warning: could not parse image manifest: %v\n", err)
		}
		return make(map[string]string)
	}

	// Validate manifest URLs — only trust http/https schemes
	manifest := make(map[string]string, len(raw))
	for filename, url := range raw {
		if strings.HasPrefix(url, "https://") || strings.HasPrefix(url, "http://") {
			manifest[filename] = url
		} else if output.GetFormat() != output.FormatJSON {
			fmt.Printf("Warning: skipping manifest entry with invalid URL scheme: %s\n", filename)
		}
	}

	return manifest
}

// imageBlockNode creates a Tiptap imageBlock node matching the app's format.
func imageBlockNode(src, alt string) map[string]interface{} {
	return map[string]interface{}{
		"type": "imageBlock",
		"attrs": map[string]interface{}{
			"src":   src,
			"alt":   alt,
			"width": "600",
			"align": "center",
		},
	}
}

// extractH1Title extracts the first H1 heading as a title and returns the remaining markdown.
// This matches how the app parses lesson/intro/assignment files: H1 → title field, rest → content_json.
func extractH1Title(md string) (title string, body string) {
	lines := strings.Split(md, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") && !strings.HasPrefix(trimmed, "## ") {
			title = strings.TrimPrefix(trimmed, "# ")
			// Body is everything after the H1 line, trimmed
			body = strings.TrimSpace(strings.Join(lines[i+1:], "\n"))
			return title, body
		}
		// Skip blank lines at the top before the H1
		if trimmed != "" {
			// Non-blank, non-H1 line — no title found
			break
		}
	}
	// No H1 found, entire content is the body
	return "", strings.TrimSpace(md)
}

func parseSLTsFromOutline(content string) []string {
	var slts []string
	lines := strings.Split(content, "\n")
	inSLTSection := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Look for ## SLTs or ## SLT heading (case-insensitive, exact match per format guide)
		lower := strings.ToLower(trimmed)
		if lower == "## slts" || lower == "## slt" {
			inSLTSection = true
			continue
		}

		// Stop at next heading
		if inSLTSection && strings.HasPrefix(trimmed, "#") {
			break
		}

		// Parse numbered list items
		if inSLTSection {
			if matches := sltLineRe.FindStringSubmatch(trimmed); len(matches) > 1 {
				slts = append(slts, matches[1])
			}
		}
	}

	return slts
}

func extractLessonNumber(path string) int {
	base := filepath.Base(path)
	if matches := lessonNumRe.FindStringSubmatch(base); len(matches) > 1 {
		num, _ := strconv.Atoi(matches[1])
		return num
	}
	return 0
}

// markdownToTiptap converts Markdown to Tiptap JSON using goldmark.
// The manifest maps local image filenames (e.g. "diagram.png") to their original CDN URLs.
// Manifest URLs are resolved via string replacement before parsing, so the AST converter
// only ever sees fully-qualified URLs.
func markdownToTiptap(md string, manifest map[string]string) (map[string]interface{}, error) {
	// Pre-process: replace local asset paths with original CDN URLs from manifest
	md = resolveManifestPaths(md, manifest)

	// Create goldmark parser with GFM extensions
	gm := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
	)

	reader := text.NewReader([]byte(md))
	doc := gm.Parser().Parse(reader)

	// Convert AST to Tiptap
	content := convertNode(doc, []byte(md))

	return map[string]interface{}{
		"type":    "doc",
		"content": content,
	}, nil
}

// resolveManifestPaths replaces local asset references with original CDN URLs.
// e.g. "assets/diagram.png" → "https://cdn.andamio.io/images/abc/diagram.png"
func resolveManifestPaths(md string, manifest map[string]string) string {
	if len(manifest) == 0 {
		return md
	}
	for filename, url := range manifest {
		md = strings.ReplaceAll(md, "assets/"+filename, url)
	}
	return md
}

func convertNode(n ast.Node, source []byte) []interface{} {
	var result []interface{}

	for child := n.FirstChild(); child != nil; child = child.NextSibling() {
		node := convertSingleNode(child, source)
		if node != nil {
			result = append(result, node)
		}
	}

	return result
}

func convertSingleNode(n ast.Node, source []byte) map[string]interface{} {
	switch node := n.(type) {
	case *ast.Paragraph:
		// Check for solo-image paragraph → imageBlock (matches app behavior)
		if node.ChildCount() == 1 {
			if img, ok := node.FirstChild().(*ast.Image); ok {
				src := string(img.Destination)
				alt := string(img.Text(source))
				if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") {
					return imageBlockNode(src, alt)
				}
				// Unresolved local image — placeholder
				if alt == "" {
					alt = "[image]"
				}
				return map[string]interface{}{
					"type": "paragraph",
					"content": []interface{}{
						map[string]interface{}{
							"type": "text",
							"text": fmt.Sprintf("[Image: %s]", alt),
						},
					},
				}
			}
		}

		content := convertInlineContent(node, source)
		if len(content) == 0 {
			return nil
		}
		return map[string]interface{}{
			"type":    "paragraph",
			"content": content,
		}

	case *ast.TextBlock:
		// TextBlock is used inside tight list items (no blank lines between items).
		// Treat the same as a paragraph so bullet text isn't silently dropped.
		content := convertInlineContent(node, source)
		if len(content) == 0 {
			return nil
		}
		return map[string]interface{}{
			"type":    "paragraph",
			"content": content,
		}

	case *ast.Heading:
		content := convertInlineContent(node, source)
		return map[string]interface{}{
			"type": "heading",
			"attrs": map[string]interface{}{
				"level": node.Level,
			},
			"content": content,
		}

	case *ast.List:
		listType := "bulletList"
		if node.IsOrdered() {
			listType = "orderedList"
		}
		items := convertNode(node, source)
		return map[string]interface{}{
			"type":    listType,
			"content": items,
		}

	case *ast.ListItem:
		// List items contain paragraphs and possibly nested lists
		content := convertNode(node, source)
		return map[string]interface{}{
			"type":    "listItem",
			"content": content,
		}

	case *ast.FencedCodeBlock:
		var code strings.Builder
		lines := node.Lines()
		for i := 0; i < lines.Len(); i++ {
			line := lines.At(i)
			code.Write(line.Value(source))
		}
		lang := string(node.Language(source))
		return map[string]interface{}{
			"type": "codeBlock",
			"attrs": map[string]interface{}{
				"language": lang,
			},
			"content": []interface{}{
				map[string]interface{}{
					"type": "text",
					"text": strings.TrimSuffix(code.String(), "\n"),
				},
			},
		}

	case *ast.CodeBlock:
		var code strings.Builder
		lines := node.Lines()
		for i := 0; i < lines.Len(); i++ {
			line := lines.At(i)
			code.Write(line.Value(source))
		}
		return map[string]interface{}{
			"type":  "codeBlock",
			"attrs": map[string]interface{}{},
			"content": []interface{}{
				map[string]interface{}{
					"type": "text",
					"text": strings.TrimSuffix(code.String(), "\n"),
				},
			},
		}

	case *ast.Blockquote:
		content := convertNode(node, source)
		return map[string]interface{}{
			"type":    "blockquote",
			"content": content,
		}

	case *ast.ThematicBreak:
		return map[string]interface{}{
			"type": "horizontalRule",
		}

	case *ast.Image:
		src := string(node.Destination)
		alt := string(node.Text(source))

		// Block-level image → imageBlock (matches app behavior)
		if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") {
			return imageBlockNode(src, alt)
		}

		// Unresolved local image — placeholder
		if alt == "" {
			alt = "[image]"
		}
		return map[string]interface{}{
			"type": "paragraph",
			"content": []interface{}{
				map[string]interface{}{
					"type": "text",
					"text": fmt.Sprintf("[Image: %s]", alt),
				},
			},
		}

	default:
		// For unhandled block nodes, try to convert children
		if n.HasChildren() {
			content := convertNode(n, source)
			if len(content) > 0 {
				if m, ok := content[0].(map[string]interface{}); ok {
					return m
				}
			}
		}
	}

	return nil
}

func convertInlineContent(n ast.Node, source []byte) []interface{} {
	var result []interface{}

	for child := n.FirstChild(); child != nil; child = child.NextSibling() {
		nodes := convertInlineNode(child, source)
		result = append(result, nodes...)
	}

	return result
}

func convertInlineNode(n ast.Node, source []byte) []interface{} {
	switch node := n.(type) {
	case *ast.Text:
		text := string(node.Segment.Value(source))
		if text == "" {
			return nil
		}
		result := map[string]interface{}{
			"type": "text",
			"text": text,
		}
		// Handle soft line breaks
		if node.SoftLineBreak() {
			result["text"] = text + " "
		}
		return []interface{}{result}

	case *ast.String:
		text := string(node.Value)
		if text == "" {
			return nil
		}
		return []interface{}{
			map[string]interface{}{
				"type": "text",
				"text": text,
			},
		}

	case *ast.CodeSpan:
		text := string(node.Text(source))
		return []interface{}{
			map[string]interface{}{
				"type": "text",
				"text": text,
				"marks": []interface{}{
					map[string]interface{}{"type": "code"},
				},
			},
		}

	case *ast.Emphasis:
		// Collect child text with marks
		var children []interface{}
		for child := node.FirstChild(); child != nil; child = child.NextSibling() {
			nodes := convertInlineNode(child, source)
			children = append(children, nodes...)
		}

		// Apply emphasis mark (level 1 = italic, level 2 = bold)
		markType := "italic"
		if node.Level == 2 {
			markType = "bold"
		}

		for _, c := range children {
			if m, ok := c.(map[string]interface{}); ok {
				marks, _ := m["marks"].([]interface{})
				marks = append(marks, map[string]interface{}{"type": markType})
				m["marks"] = marks
			}
		}

		return children

	case *extast.Strikethrough:
		// Collect child text with strike mark
		var children []interface{}
		for child := node.FirstChild(); child != nil; child = child.NextSibling() {
			nodes := convertInlineNode(child, source)
			children = append(children, nodes...)
		}

		// Apply strike mark
		for _, c := range children {
			if m, ok := c.(map[string]interface{}); ok {
				marks, _ := m["marks"].([]interface{})
				marks = append(marks, map[string]interface{}{"type": "strike"})
				m["marks"] = marks
			}
		}

		return children

	case *ast.Link:
		// Collect child text
		var textContent string
		for child := node.FirstChild(); child != nil; child = child.NextSibling() {
			if t, ok := child.(*ast.Text); ok {
				textContent += string(t.Segment.Value(source))
			}
		}

		href := string(node.Destination)
		return []interface{}{
			map[string]interface{}{
				"type": "text",
				"text": textContent,
				"marks": []interface{}{
					map[string]interface{}{
						"type": "link",
						"attrs": map[string]interface{}{
							"href": href,
						},
					},
				},
			},
		}

	case *ast.AutoLink:
		href := string(node.URL(source))
		return []interface{}{
			map[string]interface{}{
				"type": "text",
				"text": href,
				"marks": []interface{}{
					map[string]interface{}{
						"type": "link",
						"attrs": map[string]interface{}{
							"href": href,
						},
					},
				},
			},
		}

	case *ast.Image:
		src := string(node.Destination)
		alt := string(node.Text(source))

		// Inline image with resolved URL → imageBlock
		if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") {
			return []interface{}{imageBlockNode(src, alt)}
		}

		// Unresolved local image — placeholder text
		return []interface{}{
			map[string]interface{}{
				"type": "text",
				"text": fmt.Sprintf("[Image: %s]", alt),
			},
		}

	case *ast.RawHTML:
		// Skip raw HTML
		return nil

	default:
		// For unknown inline nodes, try to get text content
		if n.HasChildren() {
			var result []interface{}
			for child := n.FirstChild(); child != nil; child = child.NextSibling() {
				nodes := convertInlineNode(child, source)
				result = append(result, nodes...)
			}
			return result
		}
	}

	return nil
}

// ExistingModuleData holds the current state of the module from the API
type ExistingModuleData struct {
	Status       string
	SLTCount     int                            // number of existing SLTs
	Lessons      map[int]map[string]interface{} // slt_index → lesson fields
	Introduction map[string]interface{}
	Assignment   map[string]interface{}
}

// fetchExistingModule gets the current module state from the teacher endpoint.
// This is used to merge existing metadata (titles, descriptions, image_url, video_url)
// with the new content being imported.
func fetchExistingModule(c *client.Client, courseID, moduleCode string) (*ExistingModuleData, error) {
	var resp map[string]interface{}
	reqBody := map[string]string{"course_id": courseID}
	if err := c.Post("/api/v2/course/teacher/course-modules/list", reqBody, &resp); err != nil {
		return nil, fmt.Errorf("failed to fetch module: %w", err)
	}

	modules, ok := resp["data"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected response format")
	}

	for _, m := range modules {
		mod, ok := m.(map[string]interface{})
		if !ok {
			continue
		}
		content, ok := mod["content"].(map[string]interface{})
		if !ok {
			continue
		}
		code, ok := content["course_module_code"].(string)
		if !ok || code != moduleCode {
			continue
		}

		existing := &ExistingModuleData{
			Lessons: make(map[int]map[string]interface{}),
		}

		if status, ok := content["module_status"].(string); ok {
			existing.Status = status
		}

		// Extract existing SLT count and lesson metadata keyed by slt_index
		if slts, ok := content["slts"].([]interface{}); ok {
			existing.SLTCount = len(slts)
			for i, slt := range slts {
				sltMap, ok := slt.(map[string]interface{})
				if !ok {
					continue
				}
				if lesson, ok := sltMap["lesson"].(map[string]interface{}); ok {
					existing.Lessons[i+1] = lesson
				}
			}
		}

		// Extract existing introduction metadata
		if intro, ok := content["introduction"].(map[string]interface{}); ok {
			existing.Introduction = intro
		}

		// Extract existing assignment metadata
		if assign, ok := content["assignment"].(map[string]interface{}); ok {
			existing.Assignment = assign
		}

		return existing, nil
	}

	return nil, fmt.Errorf("%w: '%s' in course '%s'", errModuleNotFound, moduleCode, courseID)
}

func updateModuleContent(c *client.Client, courseID string, data *ImportData, existing *ExistingModuleData, sltsLocked bool, dryRun bool) (map[string]interface{}, error) {
	// Build lessons payload with H1-extracted titles
	lessons := make([]map[string]interface{}, len(data.Lessons))
	for i, lesson := range data.Lessons {
		l := map[string]interface{}{
			"slt_index":    lesson.Index,
			"content_json": lesson.TiptapJSON,
		}

		// Use H1-extracted title; fall back to existing title
		if lesson.Title != "" {
			l["title"] = lesson.Title
		} else if existingLesson, ok := existing.Lessons[lesson.Index]; ok {
			if v, ok := existingLesson["title"].(string); ok && v != "" {
				l["title"] = v
			}
		}

		// Preserve existing metadata not in markdown files
		if existingLesson, ok := existing.Lessons[lesson.Index]; ok {
			for _, field := range []string{"description", "image_url", "video_url"} {
				if v, ok := existingLesson[field]; ok && v != nil && v != "" {
					l[field] = v
				}
			}
		}

		lessons[i] = l
	}

	payload := map[string]interface{}{
		"course_id":          courseID,
		"course_module_code": data.ModuleCode,
		"title":              data.Title,
	}

	// Only include lessons if files were provided (omitting = preserve existing per API contract)
	if len(lessons) > 0 {
		payload["lessons"] = lessons
	}

	// Only include SLTs if they're not locked (module is in DRAFT status)
	if !sltsLocked {
		slts := make([]map[string]interface{}, len(data.SLTs))
		for i, sltText := range data.SLTs {
			slt := map[string]interface{}{
				"slt_text": sltText,
			}
			// Include slt_index only when updating existing SLTs.
			// Omitting slt_index triggers creation (API contract).
			if existing.SLTCount > 0 {
				slt["slt_index"] = i + 1
			}
			slts[i] = slt
		}
		payload["slts"] = slts
	}

	// Build introduction — H1 title from file, preserve existing metadata
	if data.Introduction != nil {
		intro := map[string]interface{}{
			"content_json": data.Introduction.TiptapJSON,
		}
		if data.Introduction.Title != "" {
			intro["title"] = data.Introduction.Title
		} else if existing.Introduction != nil {
			if v, ok := existing.Introduction["title"].(string); ok && v != "" {
				intro["title"] = v
			}
		}
		if existing.Introduction != nil {
			for _, field := range []string{"description", "image_url", "video_url"} {
				if v, ok := existing.Introduction[field]; ok && v != nil && v != "" {
					intro[field] = v
				}
			}
		}
		payload["introduction"] = intro
	}

	// Build assignment — H1 title from file, preserve existing metadata
	if data.Assignment != nil {
		assign := map[string]interface{}{
			"content_json": data.Assignment.TiptapJSON,
		}
		if data.Assignment.Title != "" {
			assign["title"] = data.Assignment.Title
		} else if existing.Assignment != nil {
			if v, ok := existing.Assignment["title"].(string); ok && v != "" {
				assign["title"] = v
			}
		}
		if existing.Assignment != nil {
			for _, field := range []string{"description", "image_url", "video_url"} {
				if v, ok := existing.Assignment[field]; ok && v != nil && v != "" {
					assign[field] = v
				}
			}
		}
		payload["assignment"] = assign
	}

	// Dry-run: dump payload as JSON instead of sending
	if dryRun {
		payloadJSON, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("failed to marshal payload: %w", err)
		}
		if output.GetFormat() != output.FormatJSON {
			fmt.Println("Dry-run payload (not sent):")
			fmt.Println(string(payloadJSON))
		}
		return map[string]interface{}{
			"dry_run": true,
			"payload": payload,
			"changes": map[string]interface{}{},
		}, nil
	}

	var resp map[string]interface{}
	if err := c.Post("/api/v2/course/teacher/course-module/update", payload, &resp); err != nil {
		return nil, fmt.Errorf("failed to update module: %w", err)
	}

	return resp, nil
}

// uploadImage uploads a single image file to the app's /api/upload endpoint.
// Returns the public CDN URL on success.
func uploadImage(cfg *config.Config, filePath string) (string, error) {
	// Derive app URL from API URL (same pattern as user.go OAuth flow)
	appURL := strings.Replace(cfg.BaseURL, ".api.", ".app.", 1)
	uploadURL := appURL + "/api/upload"

	// Read file
	fileData, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read %s: %w", filepath.Base(filePath), err)
	}

	// 5MB limit (matches app validation)
	if len(fileData) > 5*1024*1024 {
		return "", fmt.Errorf("%s exceeds 5MB limit (%d bytes)", filepath.Base(filePath), len(fileData))
	}

	// Determine MIME type from extension
	mimeType := ""
	switch strings.ToLower(filepath.Ext(filePath)) {
	case ".png":
		mimeType = "image/png"
	case ".jpg", ".jpeg":
		mimeType = "image/jpeg"
	case ".gif":
		mimeType = "image/gif"
	case ".webp":
		mimeType = "image/webp"
	default:
		return "", fmt.Errorf("%s: unsupported image type (allowed: PNG, JPG, GIF, WebP)", filepath.Base(filePath))
	}

	// Build multipart form with correct Content-Type
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	partHeader := make(textproto.MIMEHeader)
	safeName := strings.ReplaceAll(filepath.Base(filePath), `"`, `_`)
	partHeader.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="%s"`, safeName))
	partHeader.Set("Content-Type", mimeType)
	part, err := writer.CreatePart(partHeader)
	if err != nil {
		return "", err
	}
	if _, err := part.Write(fileData); err != nil {
		return "", err
	}
	writer.Close()

	// Send request
	req, err := http.NewRequest("POST", uploadURL, &buf)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+cfg.UserJWT)

	httpClient := &http.Client{Timeout: 30 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("upload request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("upload failed (%d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		URL         string `json:"url"`
		Key         string `json:"key"`
		Size        int    `json:"size"`
		ContentType string `json:"contentType"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to parse upload response: %w", err)
	}

	return result.URL, nil
}

// uploadNewImages uploads images not in the manifest and adds their URLs to the manifest.
// Returns the number of images successfully uploaded.
func uploadNewImages(cfg *config.Config, assetsDir string, newImages []string, manifest map[string]string) (int, []string) {
	var uploaded int
	var failed []string

	for _, filename := range newImages {
		filePath := filepath.Join(assetsDir, filename)
		url, err := uploadImage(cfg, filePath)
		if err != nil {
			failed = append(failed, fmt.Sprintf("%s: %v", filename, err))
			continue
		}
		manifest[filename] = url
		uploaded++
	}

	return uploaded, failed
}
