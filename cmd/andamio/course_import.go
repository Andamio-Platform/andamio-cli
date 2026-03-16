package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/Andamio-Platform/andamio-cli/internal/config"
	"github.com/adrg/frontmatter"
	"github.com/spf13/cobra"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

func init() {
	courseCmd.AddCommand(courseImportCmd)
	courseImportCmd.Flags().String("course-id", "", "Course ID to import into (required)")
	courseImportCmd.MarkFlagRequired("course-id")
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

Note: Image upload is not yet supported. Images in assets/ will be skipped with a warning.

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
	Introduction  map[string]interface{}
	Assignment    map[string]interface{}
	ImageWarnings []string
}

// LessonImport holds a single lesson's data
type LessonImport struct {
	Index      int
	TiptapJSON map[string]interface{}
}

// OutlineFrontmatter is the YAML frontmatter structure in outline.md
type OutlineFrontmatter struct {
	Title string `yaml:"title"`
	Code  string `yaml:"code"`
}

func runCourseImport(cmd *cobra.Command, args []string) error {
	moduleDir := args[0]
	courseID, _ := cmd.Flags().GetString("course-id")

	// Validate directory exists
	info, err := os.Stat(moduleDir)
	if err != nil {
		return fmt.Errorf("directory not found: %s", moduleDir)
	}
	if !info.IsDir() {
		return fmt.Errorf("not a directory: %s", moduleDir)
	}

	fmt.Printf("Reading module from %s...\n", moduleDir)

	// Read and parse the compiled module
	data, err := readCompiledModule(moduleDir)
	if err != nil {
		return err
	}

	// Warn about images
	if len(data.ImageWarnings) > 0 {
		fmt.Printf("Warning: %d image(s) in assets/ cannot be uploaded (not yet supported):\n", len(data.ImageWarnings))
		for _, img := range data.ImageWarnings {
			fmt.Printf("  - %s\n", img)
		}
		fmt.Println()
	}

	fmt.Printf("Importing module %s (%s) with %d lessons...\n", data.ModuleCode, data.Title, len(data.Lessons))

	// Load config and create client
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	c := client.New(cfg)

	// Update the module via API
	if err := updateModuleContent(c, courseID, data); err != nil {
		return err
	}

	fmt.Printf("Successfully imported module %s to course %s\n", data.ModuleCode, courseID)
	return nil
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

		tiptap, err := markdownToTiptap(string(content))
		if err != nil {
			return nil, fmt.Errorf("failed to convert %s: %w", filepath.Base(lessonFile), err)
		}

		data.Lessons = append(data.Lessons, LessonImport{
			Index:      lessonNum,
			TiptapJSON: tiptap,
		})
	}

	// Verify lesson count matches SLT count
	if len(data.Lessons) != len(data.SLTs) {
		return nil, fmt.Errorf("found %d lesson files but outline lists %d SLTs", len(data.Lessons), len(data.SLTs))
	}

	// Read introduction.md if exists
	introPath := filepath.Join(dir, "introduction.md")
	if introBytes, err := os.ReadFile(introPath); err == nil && len(introBytes) > 0 {
		tiptap, err := markdownToTiptap(string(introBytes))
		if err != nil {
			return nil, fmt.Errorf("failed to convert introduction.md: %w", err)
		}
		data.Introduction = tiptap
	}

	// Read assignment.md if exists
	assignPath := filepath.Join(dir, "assignment.md")
	if assignBytes, err := os.ReadFile(assignPath); err == nil && len(assignBytes) > 0 {
		tiptap, err := markdownToTiptap(string(assignBytes))
		if err != nil {
			return nil, fmt.Errorf("failed to convert assignment.md: %w", err)
		}
		data.Assignment = tiptap
	}

	// Check for images in assets/
	assetsDir := filepath.Join(dir, "assets")
	if info, err := os.Stat(assetsDir); err == nil && info.IsDir() {
		files, _ := os.ReadDir(assetsDir)
		for _, f := range files {
			if !f.IsDir() {
				data.ImageWarnings = append(data.ImageWarnings, f.Name())
			}
		}
	}

	return data, nil
}

func parseSLTsFromOutline(content string) []string {
	var slts []string
	lines := strings.Split(content, "\n")
	inSLTSection := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Look for ## SLTs heading
		if strings.HasPrefix(trimmed, "## SLT") {
			inSLTSection = true
			continue
		}

		// Stop at next heading
		if inSLTSection && strings.HasPrefix(trimmed, "#") {
			break
		}

		// Parse numbered list items
		if inSLTSection {
			// Match "1. text" or "1) text"
			re := regexp.MustCompile(`^\d+[\.\)]\s+(.+)$`)
			if matches := re.FindStringSubmatch(trimmed); len(matches) > 1 {
				slts = append(slts, matches[1])
			}
		}
	}

	return slts
}

func extractLessonNumber(path string) int {
	base := filepath.Base(path)
	// Extract number from "lesson-N.md"
	re := regexp.MustCompile(`lesson-(\d+)\.md`)
	if matches := re.FindStringSubmatch(base); len(matches) > 1 {
		num, _ := strconv.Atoi(matches[1])
		return num
	}
	return 0
}

// markdownToTiptap converts Markdown to Tiptap JSON using goldmark
func markdownToTiptap(md string) (map[string]interface{}, error) {
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
		// Images can't be uploaded yet - skip with warning handled elsewhere
		// Return a paragraph with the alt text as placeholder
		alt := string(node.Text(source))
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
				return content[0].(map[string]interface{})
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

		// For external URLs, create proper image node
		// For local assets/, we'll skip (they can't be uploaded)
		if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") {
			return []interface{}{
				map[string]interface{}{
					"type": "image",
					"attrs": map[string]interface{}{
						"src": src,
						"alt": alt,
					},
				},
			}
		}

		// Local image - return placeholder text
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

func updateModuleContent(c *client.Client, courseID string, data *ImportData) error {
	// Build lessons payload
	lessons := make([]map[string]interface{}, len(data.Lessons))
	for i, lesson := range data.Lessons {
		lessons[i] = map[string]interface{}{
			"slt_index":    lesson.Index,
			"content_json": lesson.TiptapJSON,
		}
	}

	// Build SLTs payload
	slts := make([]map[string]interface{}, len(data.SLTs))
	for i, sltText := range data.SLTs {
		slts[i] = map[string]interface{}{
			"slt_index": i + 1,
			"slt_text":  sltText,
		}
	}

	payload := map[string]interface{}{
		"course_id":          courseID,
		"course_module_code": data.ModuleCode,
		"title":              data.Title,
		"slts":               slts,
		"lessons":            lessons,
	}

	if data.Introduction != nil {
		payload["introduction"] = data.Introduction
	}
	if data.Assignment != nil {
		payload["assignment"] = data.Assignment
	}

	var resp map[string]interface{}
	if err := c.Post("/api/v2/course/teacher/course-module/update", payload, &resp); err != nil {
		return fmt.Errorf("failed to update module: %w", err)
	}

	return nil
}
