package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Andamio-Platform/andamio-cli/internal/apierr"
	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/Andamio-Platform/andamio-cli/internal/config"
	"github.com/Andamio-Platform/andamio-cli/internal/output"
	"github.com/spf13/cobra"
)

var courseExportCmd = &cobra.Command{
	Use:   "export [course-id] <module-code>",
	Short: "Export a course module to compiled/ format",
	Long: `Export a course module to the compiled/ directory format used by lesson-coach.

This creates a directory structure that can be edited locally and re-imported:
  compiled/<course-slug>/<module-code>/
  ├── outline.md          # Module metadata and SLTs
  ├── introduction.md     # Module introduction (if present)
  ├── lesson-1.md         # Lesson for SLT 1
  ├── lesson-N.md         # Lesson for SLT N
  ├── assignment.md       # Module assignment (if present)
  └── assets/             # Downloaded images

The course can be specified by ID (first arg) or by name (--course flag):
  andamio course export <course-id> <module-code>
  andamio course export <module-code> --course "Intro to Cardano"

Requires user authentication via 'andamio user login'.`,
	Args: cobra.RangeArgs(1, 2),
	PreRunE: func(cmd *cobra.Command, args []string) error {
		// Check user auth
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		if !cfg.HasUserAuth() {
			return &apierr.AuthError{Message: "not authenticated. Run 'andamio user login' first"}
		}
		return nil
	},
	RunE: runCourseExport,
}

func init() {
	courseCmd.AddCommand(courseExportCmd)
	courseExportCmd.Flags().String("output-dir", "", "Output directory (default: ./compiled/<course-slug>/<module-code>/)")
	courseExportCmd.Flags().Bool("force", false, "Overwrite existing directory")
	courseExportCmd.Flags().String("course", "", "Course name or substring (alternative to course-id arg)")
}

// ExportResult holds the result of an export operation for structured output
type ExportResult struct {
	CourseID   string   `json:"course_id"`
	CourseSlug string   `json:"course_slug"`
	ModuleCode string   `json:"module_code"`
	Title      string   `json:"title"`
	Status     string   `json:"status"`
	OutputDir  string   `json:"output_dir"`
	Files      []string `json:"files"`
	Images     int      `json:"images"`
	SLTs       []string `json:"slts"`
}

func runCourseExport(cmd *cobra.Command, args []string) error {
	isJSON := output.GetFormat() == output.FormatJSON

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	c := client.New(cfg)

	var courseID, moduleCode string
	if len(args) == 2 {
		// export <course-id> <module-code>
		courseID = args[0]
		moduleCode = args[1]
	} else {
		// export <module-code> --course "Name"
		moduleCode = args[0]
		courseID, err = resolveCourseID(c, "", cmd)
		if err != nil {
			return err
		}
	}

	// Single teacher endpoint fetches everything
	if !isJSON {
		fmt.Fprintf(os.Stderr, "Fetching module %s from course %s...\n", moduleCode, courseID)
	}
	moduleData, err := fetchModuleData(c, courseID, moduleCode)
	if err != nil {
		return err
	}

	if !isJSON && len(moduleData.SLTs) == 0 {
		fmt.Fprintf(os.Stderr, "Warning: module %s (%s) has no SLTs defined. Exported outline will be empty.\n", moduleCode, moduleData.Status)
	}

	// Determine output directory
	outputDir, _ := cmd.Flags().GetString("output-dir")
	if outputDir == "" {
		outputDir = filepath.Join("compiled", moduleData.CourseSlug, moduleCode)
	}

	// Check if output directory exists
	force, _ := cmd.Flags().GetBool("force")
	if info, err := os.Stat(outputDir); err == nil && info.IsDir() {
		if !force {
			return fmt.Errorf("output directory exists: %s. Use --force to overwrite", outputDir)
		}
		if !isJSON {
			fmt.Fprintf(os.Stderr, "Warning: overwriting existing directory %s\n", outputDir)
		}
	}

	// Write compiled module
	result, err := writeCompiledModule(outputDir, moduleData)
	if err != nil {
		return err
	}

	// Build structured result
	sltTexts := make([]string, len(moduleData.SLTs))
	for i, slt := range moduleData.SLTs {
		sltTexts[i] = slt.Text
	}

	exportResult := ExportResult{
		CourseID:   courseID,
		CourseSlug: moduleData.CourseSlug,
		ModuleCode: moduleCode,
		Title:      moduleData.Title,
		Status:     moduleData.Status,
		OutputDir:  outputDir,
		Files:      result.Files,
		Images:     result.Images,
		SLTs:       sltTexts,
	}

	if isJSON {
		return output.PrintJSON(exportResult)
	}

	// Text TUI output
	fmt.Println()
	fmt.Printf("  Module:  %s (%s)\n", moduleData.Title, moduleCode)
	fmt.Printf("  Status:  %s\n", moduleData.Status)
	fmt.Printf("  Path:    %s\n", outputDir)
	fmt.Println()
	fmt.Println("  Files:")
	for _, f := range result.Files {
		fmt.Printf("    %s\n", f)
	}
	if result.Images > 0 {
		fmt.Printf("\n  Images:  %d downloaded to assets/\n", result.Images)
	}
	fmt.Println()
	return nil
}

// ModuleData holds all the data for a module export
type ModuleData struct {
	CourseID     string
	CourseSlug   string
	ModuleCode   string
	Title        string
	Status       string
	SLTs         []SLTData
	Introduction map[string]interface{}
	Assignment   map[string]interface{}
	ImageURLs    []string
}

// SLTData holds SLT info and lesson content
type SLTData struct {
	Index  int
	Text   string
	Lesson map[string]interface{}
}

var slugifyRe = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(s string) string {
	s = strings.ToLower(s)
	s = slugifyRe.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return s
}

func fetchModuleData(c *client.Client, courseID, moduleCode string) (*ModuleData, error) {
	data := &ModuleData{
		CourseID:   courseID,
		ModuleCode: moduleCode,
	}

	// Fetch course slug from teacher courses list
	var coursesResp map[string]interface{}
	if err := c.Post("/api/v2/course/teacher/courses/list", nil, &coursesResp); err == nil {
		if courses, ok := coursesResp["data"].([]interface{}); ok {
			for _, course := range courses {
				courseMap, ok := course.(map[string]interface{})
				if !ok {
					continue
				}
				if id, ok := courseMap["course_id"].(string); ok && id == courseID {
					if content, ok := courseMap["content"].(map[string]interface{}); ok {
						if title, ok := content["title"].(string); ok {
							data.CourseSlug = slugify(title)
						}
					}
					break
				}
			}
		}
	}
	if data.CourseSlug == "" {
		data.CourseSlug = courseID
	}

	// Fetch all modules with full content (draft + on-chain) in one call
	var modulesResp map[string]interface{}
	reqBody := map[string]string{"course_id": courseID}
	if err := c.Post("/api/v2/course/teacher/course-modules/list", reqBody, &modulesResp); err != nil {
		return nil, fmt.Errorf("failed to fetch modules: %w", err)
	}

	// Find the target module in the response
	modules, ok := modulesResp["data"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected response format: missing data array")
	}

	var moduleContent map[string]interface{}
	for _, m := range modules {
		mod, ok := m.(map[string]interface{})
		if !ok {
			continue
		}
		content, ok := mod["content"].(map[string]interface{})
		if !ok {
			continue
		}
		if code, ok := content["course_module_code"].(string); ok && code == moduleCode {
			moduleContent = content
			break
		}
	}

	if moduleContent == nil {
		return nil, fmt.Errorf("module '%s' not found in course '%s'", moduleCode, courseID)
	}

	// Extract title and status
	if title, ok := moduleContent["title"].(string); ok {
		data.Title = title
	}
	if status, ok := moduleContent["module_status"].(string); ok {
		data.Status = status
	}

	// Extract SLTs with embedded lessons from content.slts[]
	sltsData, _ := moduleContent["slts"].([]interface{})
	data.SLTs = make([]SLTData, len(sltsData))

	for i, sltItem := range sltsData {
		sltMap, ok := sltItem.(map[string]interface{})
		if !ok {
			continue
		}

		sltIndex := i + 1
		sltText := ""
		if text, ok := sltMap["slt_text"].(string); ok {
			sltText = text
		}

		// Extract embedded lesson content (LessonV2 with content_json + title)
		var lessonContent map[string]interface{}
		var lessonTitle string
		if lesson, ok := sltMap["lesson"].(map[string]interface{}); ok {
			if contentJSON, ok := lesson["content_json"].(map[string]interface{}); ok {
				lessonContent = contentJSON
			}
			if t, ok := lesson["title"].(string); ok {
				lessonTitle = t
			}
		}

		data.SLTs[i] = SLTData{
			Index:  sltIndex,
			Text:   sltText,
			Lesson: map[string]interface{}{"content_json": lessonContent, "title": lessonTitle},
		}
	}

	// Extract introduction from module content (already inline)
	if intro, ok := moduleContent["introduction"].(map[string]interface{}); ok {
		if contentJSON, ok := intro["content_json"].(map[string]interface{}); ok {
			introTitle, _ := intro["title"].(string)
			// Wrap in the structure convertContentToMarkdown expects
			data.Introduction = map[string]interface{}{
				"data": map[string]interface{}{
					"content": map[string]interface{}{
						"content_json": contentJSON,
						"title":        introTitle,
					},
				},
			}
		}
	}

	// Extract assignment from module content (already inline)
	if assign, ok := moduleContent["assignment"].(map[string]interface{}); ok {
		if contentJSON, ok := assign["content_json"].(map[string]interface{}); ok {
			assignTitle, _ := assign["title"].(string)
			data.Assignment = map[string]interface{}{
				"data": map[string]interface{}{
					"content": map[string]interface{}{
						"content_json": contentJSON,
						"title":        assignTitle,
					},
				},
			}
		}
	}

	return data, nil
}

// WriteResult tracks what was written during export
type WriteResult struct {
	Files  []string
	Images int
}

func writeCompiledModule(outputDir string, data *ModuleData) (*WriteResult, error) {
	result := &WriteResult{}

	// Validate output directory path
	absDir, err := filepath.Abs(outputDir)
	if err != nil {
		return nil, fmt.Errorf("invalid output directory: %w", err)
	}

	// Create output directory
	if err := os.MkdirAll(absDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	// Collect image URLs for downloading
	var imageURLs []string

	// Write outline.md
	outlinePath := filepath.Join(absDir, "outline.md")
	outlineContent := generateOutline(data)
	if err := writeFileAtomic(outlinePath, []byte(outlineContent)); err != nil {
		return nil, fmt.Errorf("failed to write outline.md: %w", err)
	}
	result.Files = append(result.Files, "outline.md")

	// Write lesson files
	for _, slt := range data.SLTs {
		if slt.Lesson == nil {
			continue
		}

		filename := fmt.Sprintf("lesson-%d.md", slt.Index)
		lessonPath := filepath.Join(absDir, filename)
		lessonContent, urls := convertLessonToMarkdown(slt.Lesson)
		imageURLs = append(imageURLs, urls...)

		if err := writeFileAtomic(lessonPath, []byte(lessonContent)); err != nil {
			return nil, fmt.Errorf("failed to write %s: %w", filename, err)
		}
		result.Files = append(result.Files, filename)
	}

	// Write introduction.md if present
	if data.Introduction != nil {
		introPath := filepath.Join(absDir, "introduction.md")
		introContent, urls := convertContentToMarkdown(data.Introduction)
		imageURLs = append(imageURLs, urls...)

		if err := writeFileAtomic(introPath, []byte(introContent)); err != nil {
			return nil, fmt.Errorf("failed to write introduction.md: %w", err)
		}
		result.Files = append(result.Files, "introduction.md")
	}

	// Write assignment.md if present
	if data.Assignment != nil {
		assignPath := filepath.Join(absDir, "assignment.md")
		assignContent, urls := convertContentToMarkdown(data.Assignment)
		imageURLs = append(imageURLs, urls...)

		if err := writeFileAtomic(assignPath, []byte(assignContent)); err != nil {
			return nil, fmt.Errorf("failed to write assignment.md: %w", err)
		}
		result.Files = append(result.Files, "assignment.md")
	}

	// Download images if any
	if len(imageURLs) > 0 {
		assetsDir := filepath.Join(absDir, "assets")
		if err := os.MkdirAll(assetsDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create assets directory: %w", err)
		}

		if err := downloadImages(assetsDir, imageURLs); err != nil {
			if output.GetFormat() != output.FormatJSON {
				fmt.Fprintf(os.Stderr, "Warning: some images failed to download: %v\n", err)
			}
		}

		if err := writeImageManifest(assetsDir, imageURLs); err != nil {
			if output.GetFormat() != output.FormatJSON {
				fmt.Fprintf(os.Stderr, "Warning: failed to write image manifest: %v\n", err)
			}
		}

		// Count unique images
		seen := make(map[string]bool)
		for _, u := range imageURLs {
			seen[u] = true
		}
		result.Images = len(seen)
	}

	return result, nil
}

// writeImageManifest writes a .image-manifest.json mapping local filenames to original URLs.
// This enables the import command to restore original CDN URLs during round-trip.
func writeImageManifest(assetsDir string, urls []string) error {
	manifest := make(map[string]string)

	for _, imgURL := range urls {
		filename := filepath.Base(imgURL)
		// Remove query params from filename
		if idx := strings.Index(filename, "?"); idx != -1 {
			filename = filename[:idx]
		}
		// If duplicate filename, keep the first mapping (matches download behavior)
		if _, exists := manifest[filename]; !exists {
			manifest[filename] = imgURL
		}
	}

	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}

	manifestPath := filepath.Join(assetsDir, ".image-manifest.json")
	return writeFileAtomic(manifestPath, data)
}

func generateOutline(data *ModuleData) string {
	var buf bytes.Buffer

	// YAML frontmatter
	buf.WriteString("---\n")
	buf.WriteString(fmt.Sprintf("title: %q\n", data.Title))
	buf.WriteString(fmt.Sprintf("code: %q\n", data.ModuleCode))
	buf.WriteString("---\n\n")

	// Title heading
	buf.WriteString(fmt.Sprintf("# %s\n\n", data.Title))

	// SLTs section
	buf.WriteString("## SLTs\n\n")
	for _, slt := range data.SLTs {
		buf.WriteString(fmt.Sprintf("%d. %s\n", slt.Index, slt.Text))
	}

	return buf.String()
}

func convertLessonToMarkdown(lesson map[string]interface{}) (string, []string) {
	// Extract content_json from lesson response
	var contentJSON map[string]interface{}

	// Try direct content_json (from embedded SLT response)
	if cj, ok := lesson["content_json"].(map[string]interface{}); ok {
		contentJSON = cj
	} else if data, ok := lesson["data"].(map[string]interface{}); ok {
		// Try nested structure (from separate lesson API call)
		if lessonData, ok := data["lesson"].(map[string]interface{}); ok {
			if cj, ok := lessonData["content_json"].(map[string]interface{}); ok {
				contentJSON = cj
			}
		}
	}

	if contentJSON == nil {
		return "", nil
	}

	md, urls := tiptapToMarkdown(contentJSON)

	// Prepend title as H1 (import expects this per format guide)
	if title, ok := lesson["title"].(string); ok && title != "" {
		md = "# " + sanitizeTitle(title) + "\n\n" + md
	}

	return md, urls
}

// sanitizeTitle strips newlines and trims whitespace from a title string.
// Prevents markdown structure breakout when embedding titles as H1 headings.
func sanitizeTitle(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	return strings.TrimSpace(s)
}

func convertContentToMarkdown(resp map[string]interface{}) (string, []string) {
	// Handle introduction/assignment response structure
	// API returns: { "data": { "content": { "content_json": {...}, "title": "..." } } }
	var contentJSON map[string]interface{}
	var title string

	if data, ok := resp["data"].(map[string]interface{}); ok {
		if content, ok := data["content"].(map[string]interface{}); ok {
			if cj, ok := content["content_json"].(map[string]interface{}); ok {
				contentJSON = cj
			}
			if t, ok := content["title"].(string); ok {
				title = t
			}
		}
		// Fallback: try direct content_json on data (in case API changes)
		if contentJSON == nil {
			if cj, ok := data["content_json"].(map[string]interface{}); ok {
				contentJSON = cj
			}
		}
	}

	if contentJSON == nil {
		return "", nil
	}

	md, urls := tiptapToMarkdown(contentJSON)

	// Prepend title as H1 (import expects this per format guide)
	if title != "" {
		md = "# " + sanitizeTitle(title) + "\n\n" + md
	}

	return md, urls
}

// tiptapToMarkdown converts Tiptap JSON to Markdown
func tiptapToMarkdown(node map[string]interface{}) (string, []string) {
	var buf bytes.Buffer
	var imageURLs []string

	nodeType, _ := node["type"].(string)

	switch nodeType {
	case "doc":
		if content, ok := node["content"].([]interface{}); ok {
			for _, child := range content {
				if childMap, ok := child.(map[string]interface{}); ok {
					md, urls := tiptapToMarkdown(childMap)
					buf.WriteString(md)
					imageURLs = append(imageURLs, urls...)
				}
			}
		}

	case "paragraph":
		text, urls := renderInlineContent(node)
		imageURLs = append(imageURLs, urls...)
		buf.WriteString(text)
		buf.WriteString("\n\n")

	case "heading":
		level := 1
		if attrs, ok := node["attrs"].(map[string]interface{}); ok {
			if l, ok := attrs["level"].(float64); ok {
				level = int(l)
			}
		}
		text, urls := renderInlineContent(node)
		imageURLs = append(imageURLs, urls...)
		buf.WriteString(strings.Repeat("#", level) + " " + text + "\n\n")

	case "bulletList":
		if content, ok := node["content"].([]interface{}); ok {
			for _, item := range content {
				if itemMap, ok := item.(map[string]interface{}); ok {
					text, urls := renderListItem(itemMap, "- ")
					imageURLs = append(imageURLs, urls...)
					buf.WriteString(text)
				}
			}
		}
		buf.WriteString("\n")

	case "orderedList":
		if content, ok := node["content"].([]interface{}); ok {
			for i, item := range content {
				if itemMap, ok := item.(map[string]interface{}); ok {
					text, urls := renderListItem(itemMap, fmt.Sprintf("%d. ", i+1))
					imageURLs = append(imageURLs, urls...)
					buf.WriteString(text)
				}
			}
		}
		buf.WriteString("\n")

	case "blockquote":
		if content, ok := node["content"].([]interface{}); ok {
			for _, child := range content {
				if childMap, ok := child.(map[string]interface{}); ok {
					md, urls := tiptapToMarkdown(childMap)
					imageURLs = append(imageURLs, urls...)
					// Prefix each line with >
					for _, line := range strings.Split(strings.TrimRight(md, "\n"), "\n") {
						buf.WriteString("> " + line + "\n")
					}
				}
			}
		}
		buf.WriteString("\n")

	case "codeBlock":
		lang := ""
		if attrs, ok := node["attrs"].(map[string]interface{}); ok {
			if l, ok := attrs["language"].(string); ok {
				lang = l
			}
		}
		buf.WriteString("```" + lang + "\n")
		if content, ok := node["content"].([]interface{}); ok {
			for _, child := range content {
				if childMap, ok := child.(map[string]interface{}); ok {
					if childMap["type"] == "text" {
						if text, ok := childMap["text"].(string); ok {
							buf.WriteString(text)
						}
					}
				}
			}
		}
		buf.WriteString("\n```\n\n")

	case "horizontalRule":
		buf.WriteString("---\n\n")

	case "image", "imageBlock":
		if attrs, ok := node["attrs"].(map[string]interface{}); ok {
			src, _ := attrs["src"].(string)
			alt, _ := attrs["alt"].(string)
			if src != "" {
				imageURLs = append(imageURLs, src)
				// Convert to relative path for assets
				filename := filepath.Base(src)
				buf.WriteString(fmt.Sprintf("![%s](assets/%s)\n\n", alt, filename))
			}
		}

	case "hardBreak":
		buf.WriteString("  \n")
	}

	return buf.String(), imageURLs
}

func renderInlineContent(node map[string]interface{}) (string, []string) {
	var buf bytes.Buffer
	var imageURLs []string

	content, ok := node["content"].([]interface{})
	if !ok {
		return "", nil
	}

	for _, child := range content {
		childMap, ok := child.(map[string]interface{})
		if !ok {
			continue
		}

		childType, _ := childMap["type"].(string)

		switch childType {
		case "text":
			text, _ := childMap["text"].(string)
			text = applyMarks(text, childMap)
			buf.WriteString(text)

		case "hardBreak":
			buf.WriteString("  \n")

		case "image":
			if attrs, ok := childMap["attrs"].(map[string]interface{}); ok {
				src, _ := attrs["src"].(string)
				alt, _ := attrs["alt"].(string)
				if src != "" {
					imageURLs = append(imageURLs, src)
					filename := filepath.Base(src)
					buf.WriteString(fmt.Sprintf("![%s](assets/%s)", alt, filename))
				}
			}
		}
	}

	return buf.String(), imageURLs
}

func renderListItem(node map[string]interface{}, prefix string) (string, []string) {
	var buf bytes.Buffer
	var imageURLs []string

	content, ok := node["content"].([]interface{})
	if !ok {
		return prefix + "\n", nil
	}

	first := true
	for _, child := range content {
		childMap, ok := child.(map[string]interface{})
		if !ok {
			continue
		}

		childType, _ := childMap["type"].(string)

		if childType == "paragraph" {
			text, urls := renderInlineContent(childMap)
			imageURLs = append(imageURLs, urls...)
			if first {
				buf.WriteString(prefix + text + "\n")
				first = false
			} else {
				buf.WriteString("  " + text + "\n")
			}
		} else if childType == "bulletList" || childType == "orderedList" {
			// Nested list
			md, urls := tiptapToMarkdown(childMap)
			imageURLs = append(imageURLs, urls...)
			// Indent nested list
			for _, line := range strings.Split(strings.TrimRight(md, "\n"), "\n") {
				buf.WriteString("  " + line + "\n")
			}
		} else if childType == "image" || childType == "imageBlock" {
			// Handle images inside list items
			if attrs, ok := childMap["attrs"].(map[string]interface{}); ok {
				src, _ := attrs["src"].(string)
				alt, _ := attrs["alt"].(string)
				if src != "" {
					imageURLs = append(imageURLs, src)
					filename := filepath.Base(src)
					if first {
						buf.WriteString(prefix + fmt.Sprintf("![%s](assets/%s)\n", alt, filename))
						first = false
					} else {
						buf.WriteString(fmt.Sprintf("  ![%s](assets/%s)\n", alt, filename))
					}
				}
			}
		}
	}

	return buf.String(), imageURLs
}

func applyMarks(text string, node map[string]interface{}) string {
	marks, ok := node["marks"].([]interface{})
	if !ok {
		return text
	}

	for _, mark := range marks {
		markMap, ok := mark.(map[string]interface{})
		if !ok {
			continue
		}

		markType, _ := markMap["type"].(string)

		switch markType {
		case "bold":
			text = "**" + text + "**"
		case "italic":
			text = "*" + text + "*"
		case "strike":
			text = "~~" + text + "~~"
		case "code":
			text = "`" + text + "`"
		case "link":
			if attrs, ok := markMap["attrs"].(map[string]interface{}); ok {
				href, _ := attrs["href"].(string)
				text = "[" + text + "](" + href + ")"
			}
		}
	}

	return text
}

func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)

	// Create temp file in same directory
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()

	success := false
	defer func() {
		if !success {
			tmp.Close()
			os.Remove(tmpPath)
		}
	}()

	// Set permissions
	if err := tmp.Chmod(0644); err != nil {
		return err
	}

	// Write data
	if _, err := tmp.Write(data); err != nil {
		return err
	}

	// Sync to disk
	if err := tmp.Sync(); err != nil {
		return err
	}

	// Close before rename
	if err := tmp.Close(); err != nil {
		return err
	}

	// Atomic rename
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}

	success = true
	return nil
}

// downloadImages downloads images concurrently with a semaphore
func downloadImages(assetsDir string, urls []string) error {
	// Deduplicate URLs
	seen := make(map[string]bool)
	uniqueURLs := make([]string, 0)
	for _, u := range urls {
		if !seen[u] {
			seen[u] = true
			uniqueURLs = append(uniqueURLs, u)
		}
	}

	if len(uniqueURLs) == 0 {
		return nil
	}

	if output.GetFormat() != output.FormatJSON {
		fmt.Fprintf(os.Stderr, "Downloading %d images...\n", len(uniqueURLs))
	}

	var wg sync.WaitGroup
	sem := make(chan struct{}, 3) // 3 concurrent downloads
	errChan := make(chan error, len(uniqueURLs))

	httpClient := &http.Client{Timeout: 30 * time.Second}

	for _, imgURL := range uniqueURLs {
		// Validate URL (HTTPS only for security)
		parsed, err := url.Parse(imgURL)
		if err != nil {
			if output.GetFormat() != output.FormatJSON {
				fmt.Fprintf(os.Stderr, "Warning: invalid image URL: %s\n", imgURL)
			}
			continue
		}
		if parsed.Scheme != "https" && parsed.Scheme != "http" {
			if output.GetFormat() != output.FormatJSON {
				fmt.Fprintf(os.Stderr, "Warning: skipping non-HTTP URL: %s\n", imgURL)
			}
			continue
		}

		wg.Add(1)
		go func(imgURL string) {
			defer wg.Done()
			sem <- struct{}{}        // Acquire
			defer func() { <-sem }() // Release

			filename := filepath.Base(imgURL)
			// Remove query params from filename
			if idx := strings.Index(filename, "?"); idx != -1 {
				filename = filename[:idx]
			}

			destPath := filepath.Join(assetsDir, filename)

			resp, err := httpClient.Get(imgURL)
			if err != nil {
				errChan <- fmt.Errorf("failed to download %s: %w", imgURL, err)
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				errChan <- fmt.Errorf("failed to download %s: status %d", imgURL, resp.StatusCode)
				return
			}

			// Read body with size limit
			data, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024)) // 10MB limit
			if err != nil {
				errChan <- fmt.Errorf("failed to read %s: %w", imgURL, err)
				return
			}

			if err := writeFileAtomic(destPath, data); err != nil {
				errChan <- fmt.Errorf("failed to write %s: %w", destPath, err)
				return
			}

			if output.GetFormat() != output.FormatJSON {
				fmt.Fprintf(os.Stderr, "  Downloaded %s\n", filename)
			}
		}(imgURL)
	}

	wg.Wait()
	close(errChan)

	// Collect errors
	var errs []error
	for err := range errChan {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return fmt.Errorf("%d image(s) failed to download", len(errs))
	}

	return nil
}
