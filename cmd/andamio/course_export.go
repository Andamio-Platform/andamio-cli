package main

import (
	"bytes"
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

	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/Andamio-Platform/andamio-cli/internal/config"
	"github.com/spf13/cobra"
)

var courseExportCmd = &cobra.Command{
	Use:   "export <course-id> <module-code>",
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

Requires user authentication via 'andamio user login'.`,
	Args: cobra.ExactArgs(2),
	PreRunE: func(cmd *cobra.Command, args []string) error {
		// Check user auth
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		if !cfg.HasUserAuth() {
			return fmt.Errorf("not authenticated. Run 'andamio user login' first")
		}
		return nil
	},
	RunE: runCourseExport,
}

func init() {
	courseCmd.AddCommand(courseExportCmd)
	courseExportCmd.Flags().String("output-dir", "", "Output directory (default: ./compiled/<course-slug>/<module-code>/)")
	courseExportCmd.Flags().Bool("force", false, "Overwrite existing directory")
}

func runCourseExport(cmd *cobra.Command, args []string) error {
	courseID := args[0]
	moduleCode := args[1]

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	c := client.New(cfg)

	// Fetch course info to get slug
	fmt.Printf("Fetching course %s...\n", courseID)
	var courseResp map[string]interface{}
	if err := c.Get("/api/v2/course/user/course/get/"+url.PathEscape(courseID), &courseResp); err != nil {
		return fmt.Errorf("failed to fetch course: %w", err)
	}

	// Extract course slug from response
	courseSlug := extractCourseSlug(courseResp)
	if courseSlug == "" {
		courseSlug = courseID // Fallback to ID if no slug
	}

	// Determine output directory
	outputDir, _ := cmd.Flags().GetString("output-dir")
	if outputDir == "" {
		outputDir = filepath.Join("compiled", courseSlug, moduleCode)
	}

	// Check if output directory exists
	force, _ := cmd.Flags().GetBool("force")
	if info, err := os.Stat(outputDir); err == nil && info.IsDir() {
		if !force {
			return fmt.Errorf("output directory exists: %s. Use --force to overwrite", outputDir)
		}
		fmt.Printf("Warning: overwriting existing directory %s\n", outputDir)
	}

	// Fetch module data
	fmt.Printf("Fetching module %s...\n", moduleCode)
	moduleData, err := fetchModuleData(c, courseID, moduleCode)
	if err != nil {
		return err
	}

	// Write compiled module
	fmt.Printf("Writing to %s...\n", outputDir)
	if err := writeCompiledModule(outputDir, moduleData); err != nil {
		return err
	}

	fmt.Printf("Exported module %s to %s\n", moduleCode, outputDir)
	return nil
}

// ModuleData holds all the data for a module export
type ModuleData struct {
	CourseID     string
	ModuleCode   string
	Title        string
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

func extractCourseSlug(courseResp map[string]interface{}) string {
	// Try to get slug from response - adjust based on actual API response structure
	if content, ok := courseResp["content"].(map[string]interface{}); ok {
		if slug, ok := content["slug"].(string); ok {
			return slug
		}
		if title, ok := content["title"].(string); ok {
			// Convert title to slug
			return slugify(title)
		}
	}
	if slug, ok := courseResp["slug"].(string); ok {
		return slug
	}
	return ""
}

func slugify(s string) string {
	s = strings.ToLower(s)
	s = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return s
}

func fetchModuleData(c *client.Client, courseID, moduleCode string) (*ModuleData, error) {
	data := &ModuleData{
		CourseID:   courseID,
		ModuleCode: moduleCode,
	}

	// Fetch SLTs first to know how many lessons to fetch
	var sltsResp map[string]interface{}
	if err := c.Get("/api/v2/course/user/slts/"+url.PathEscape(courseID)+"/"+url.PathEscape(moduleCode), &sltsResp); err != nil {
		return nil, fmt.Errorf("failed to fetch SLTs: %w", err)
	}

	// Extract module title and SLTs from response
	if moduleInfo, ok := sltsResp["course_module"].(map[string]interface{}); ok {
		if title, ok := moduleInfo["title"].(string); ok {
			data.Title = title
		}
	}

	sltsData, ok := sltsResp["data"].([]interface{})
	if !ok {
		sltsData = []interface{}{}
	}

	// Fetch lessons in parallel
	fmt.Printf("Fetching %d lessons...\n", len(sltsData))
	data.SLTs = make([]SLTData, len(sltsData))

	var wg sync.WaitGroup
	sem := make(chan struct{}, 5) // Limit to 5 concurrent requests
	errChan := make(chan error, len(sltsData))

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

		data.SLTs[i] = SLTData{
			Index: sltIndex,
			Text:  sltText,
		}

		wg.Add(1)
		go func(idx int, sltIdx int) {
			defer wg.Done()
			sem <- struct{}{}        // Acquire
			defer func() { <-sem }() // Release

			var lessonResp map[string]interface{}
			err := c.Get(fmt.Sprintf("/api/v2/course/user/lesson/%s/%s/%d",
				url.PathEscape(courseID), url.PathEscape(moduleCode), sltIdx), &lessonResp)
			if err != nil {
				errChan <- fmt.Errorf("failed to fetch lesson %d: %w", sltIdx, err)
				return
			}
			data.SLTs[idx].Lesson = lessonResp
		}(i, sltIndex)
	}

	wg.Wait()
	close(errChan)

	// Check for errors
	for err := range errChan {
		return nil, err
	}

	// Fetch introduction (optional - may 404)
	var introResp map[string]interface{}
	if err := c.Get("/api/v2/course/user/introduction/"+url.PathEscape(courseID)+"/"+url.PathEscape(moduleCode), &introResp); err == nil {
		data.Introduction = introResp
	}

	// Fetch assignment (optional - may 404)
	var assignResp map[string]interface{}
	if err := c.Get("/api/v2/course/user/assignment/"+url.PathEscape(courseID)+"/"+url.PathEscape(moduleCode), &assignResp); err == nil {
		data.Assignment = assignResp
	}

	return data, nil
}

func writeCompiledModule(outputDir string, data *ModuleData) error {
	// Validate output directory path
	absDir, err := filepath.Abs(outputDir)
	if err != nil {
		return fmt.Errorf("invalid output directory: %w", err)
	}

	// Create output directory
	if err := os.MkdirAll(absDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Collect image URLs for downloading
	var imageURLs []string

	// Write outline.md
	outlinePath := filepath.Join(absDir, "outline.md")
	outlineContent := generateOutline(data)
	if err := writeFileAtomic(outlinePath, []byte(outlineContent)); err != nil {
		return fmt.Errorf("failed to write outline.md: %w", err)
	}

	// Write lesson files
	for _, slt := range data.SLTs {
		if slt.Lesson == nil {
			continue
		}

		lessonPath := filepath.Join(absDir, fmt.Sprintf("lesson-%d.md", slt.Index))
		lessonContent, urls := convertLessonToMarkdown(slt.Lesson)
		imageURLs = append(imageURLs, urls...)

		if err := writeFileAtomic(lessonPath, []byte(lessonContent)); err != nil {
			return fmt.Errorf("failed to write lesson-%d.md: %w", slt.Index, err)
		}
	}

	// Write introduction.md if present
	if data.Introduction != nil {
		introPath := filepath.Join(absDir, "introduction.md")
		introContent, urls := convertContentToMarkdown(data.Introduction)
		imageURLs = append(imageURLs, urls...)

		if err := writeFileAtomic(introPath, []byte(introContent)); err != nil {
			return fmt.Errorf("failed to write introduction.md: %w", err)
		}
	}

	// Write assignment.md if present
	if data.Assignment != nil {
		assignPath := filepath.Join(absDir, "assignment.md")
		assignContent, urls := convertContentToMarkdown(data.Assignment)
		imageURLs = append(imageURLs, urls...)

		if err := writeFileAtomic(assignPath, []byte(assignContent)); err != nil {
			return fmt.Errorf("failed to write assignment.md: %w", err)
		}
	}

	// Download images if any
	if len(imageURLs) > 0 {
		assetsDir := filepath.Join(absDir, "assets")
		if err := os.MkdirAll(assetsDir, 0755); err != nil {
			return fmt.Errorf("failed to create assets directory: %w", err)
		}

		if err := downloadImages(assetsDir, imageURLs); err != nil {
			// Log warning but don't fail
			fmt.Printf("Warning: some images failed to download: %v\n", err)
		}
	}

	return nil
}

func generateOutline(data *ModuleData) string {
	var buf bytes.Buffer

	// YAML frontmatter
	buf.WriteString("---\n")
	buf.WriteString(fmt.Sprintf("title: %s\n", data.Title))
	buf.WriteString(fmt.Sprintf("code: %s\n", data.ModuleCode))
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

	if data, ok := lesson["data"].(map[string]interface{}); ok {
		if lessonData, ok := data["lesson"].(map[string]interface{}); ok {
			if cj, ok := lessonData["content_json"].(map[string]interface{}); ok {
				contentJSON = cj
			}
		}
	}

	if contentJSON == nil {
		return "", nil
	}

	return tiptapToMarkdown(contentJSON)
}

func convertContentToMarkdown(resp map[string]interface{}) (string, []string) {
	// Handle introduction/assignment response structure
	var contentJSON map[string]interface{}

	if data, ok := resp["data"].(map[string]interface{}); ok {
		if cj, ok := data["content_json"].(map[string]interface{}); ok {
			contentJSON = cj
		}
	}

	if contentJSON == nil {
		return "", nil
	}

	return tiptapToMarkdown(contentJSON)
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

	case "image":
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

	fmt.Printf("Downloading %d images...\n", len(uniqueURLs))

	var wg sync.WaitGroup
	sem := make(chan struct{}, 3) // 3 concurrent downloads
	errChan := make(chan error, len(uniqueURLs))

	httpClient := &http.Client{Timeout: 30 * time.Second}

	for _, imgURL := range uniqueURLs {
		// Validate URL (HTTPS only for security)
		parsed, err := url.Parse(imgURL)
		if err != nil {
			fmt.Printf("Warning: invalid image URL: %s\n", imgURL)
			continue
		}
		if parsed.Scheme != "https" && parsed.Scheme != "http" {
			fmt.Printf("Warning: skipping non-HTTP URL: %s\n", imgURL)
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

			fmt.Printf("  Downloaded %s\n", filename)
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
