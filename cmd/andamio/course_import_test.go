package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestMarkdownToTiptap(t *testing.T) {
	tests := []struct {
		name     string
		markdown string
		wantType string // Top-level node type
		wantLen  int    // Number of content nodes
	}{
		{
			name:     "simple paragraph",
			markdown: "Hello world",
			wantType: "doc",
			wantLen:  1,
		},
		{
			name:     "heading level 1",
			markdown: "# Heading",
			wantType: "doc",
			wantLen:  1,
		},
		{
			name:     "heading level 2",
			markdown: "## Heading",
			wantType: "doc",
			wantLen:  1,
		},
		{
			name:     "bullet list",
			markdown: "- item 1\n- item 2",
			wantType: "doc",
			wantLen:  1,
		},
		{
			name:     "ordered list",
			markdown: "1. first\n2. second",
			wantType: "doc",
			wantLen:  1,
		},
		{
			name:     "code block",
			markdown: "```bash\necho hello\n```",
			wantType: "doc",
			wantLen:  1,
		},
		{
			name:     "blockquote",
			markdown: "> quote text",
			wantType: "doc",
			wantLen:  1,
		},
		{
			name:     "horizontal rule",
			markdown: "---",
			wantType: "doc",
			wantLen:  1,
		},
		{
			name:     "multiple paragraphs",
			markdown: "First paragraph.\n\nSecond paragraph.",
			wantType: "doc",
			wantLen:  2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := markdownToTiptap(tt.markdown, nil)
			if err != nil {
				t.Fatalf("markdownToTiptap() error = %v", err)
			}

			// Check top-level type
			if got["type"] != tt.wantType {
				t.Errorf("markdownToTiptap() type = %v, want %v", got["type"], tt.wantType)
			}

			// Check content length
			content, ok := got["content"].([]interface{})
			if !ok {
				t.Fatalf("markdownToTiptap() content is not []interface{}")
			}
			if len(content) != tt.wantLen {
				t.Errorf("markdownToTiptap() content len = %v, want %v", len(content), tt.wantLen)
			}
		})
	}
}

func TestMarkdownToTiptapHeadings(t *testing.T) {
	tests := []struct {
		markdown  string
		wantLevel int
	}{
		{"# H1", 1},
		{"## H2", 2},
		{"### H3", 3},
		{"#### H4", 4},
	}

	for _, tt := range tests {
		t.Run(tt.markdown, func(t *testing.T) {
			got, err := markdownToTiptap(tt.markdown, nil)
			if err != nil {
				t.Fatalf("markdownToTiptap() error = %v", err)
			}

			content := got["content"].([]interface{})
			heading := content[0].(map[string]interface{})

			if heading["type"] != "heading" {
				t.Errorf("expected heading, got %v", heading["type"])
			}

			attrs := heading["attrs"].(map[string]interface{})
			if attrs["level"] != tt.wantLevel {
				t.Errorf("heading level = %v, want %v", attrs["level"], tt.wantLevel)
			}
		})
	}
}

func TestMarkdownToTiptapInlineMarks(t *testing.T) {
	tests := []struct {
		name     string
		markdown string
		wantMark string
	}{
		{"bold", "**bold text**", "bold"},
		{"italic", "*italic text*", "italic"},
		{"code", "`code text`", "code"},
		{"strikethrough", "~~strike~~", "strike"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := markdownToTiptap(tt.markdown, nil)
			if err != nil {
				t.Fatalf("markdownToTiptap() error = %v", err)
			}

			content := got["content"].([]interface{})
			para := content[0].(map[string]interface{})
			paraContent := para["content"].([]interface{})

			// Find the text node with the mark
			found := false
			for _, node := range paraContent {
				textNode := node.(map[string]interface{})
				if marks, ok := textNode["marks"].([]interface{}); ok {
					for _, mark := range marks {
						markMap := mark.(map[string]interface{})
						if markMap["type"] == tt.wantMark {
							found = true
							break
						}
					}
				}
			}

			if !found {
				t.Errorf("expected mark %s not found in output", tt.wantMark)
			}
		})
	}
}

func TestMarkdownToTiptapLink(t *testing.T) {
	markdown := "[link text](https://example.com)"
	got, err := markdownToTiptap(markdown, nil)
	if err != nil {
		t.Fatalf("markdownToTiptap() error = %v", err)
	}

	content := got["content"].([]interface{})
	para := content[0].(map[string]interface{})
	paraContent := para["content"].([]interface{})

	// Find text node with link mark
	found := false
	for _, node := range paraContent {
		textNode := node.(map[string]interface{})
		if marks, ok := textNode["marks"].([]interface{}); ok {
			for _, mark := range marks {
				markMap := mark.(map[string]interface{})
				if markMap["type"] == "link" {
					attrs := markMap["attrs"].(map[string]interface{})
					if attrs["href"] == "https://example.com" {
						found = true
					}
				}
			}
		}
	}

	if !found {
		t.Errorf("expected link mark with href https://example.com not found")
	}
}

func TestMarkdownToTiptapCodeBlock(t *testing.T) {
	markdown := "```python\nprint('hello')\n```"
	got, err := markdownToTiptap(markdown, nil)
	if err != nil {
		t.Fatalf("markdownToTiptap() error = %v", err)
	}

	content := got["content"].([]interface{})
	codeBlock := content[0].(map[string]interface{})

	if codeBlock["type"] != "codeBlock" {
		t.Errorf("expected codeBlock, got %v", codeBlock["type"])
	}

	attrs := codeBlock["attrs"].(map[string]interface{})
	if attrs["language"] != "python" {
		t.Errorf("expected language python, got %v", attrs["language"])
	}
}

func TestMarkdownToTiptapLists(t *testing.T) {
	t.Run("bullet list", func(t *testing.T) {
		markdown := "- item 1\n- item 2"
		got, err := markdownToTiptap(markdown, nil)
		if err != nil {
			t.Fatalf("markdownToTiptap() error = %v", err)
		}

		content := got["content"].([]interface{})
		list := content[0].(map[string]interface{})

		if list["type"] != "bulletList" {
			t.Errorf("expected bulletList, got %v", list["type"])
		}

		listContent := list["content"].([]interface{})
		if len(listContent) != 2 {
			t.Errorf("expected 2 list items, got %d", len(listContent))
		}

		for _, item := range listContent {
			itemMap := item.(map[string]interface{})
			if itemMap["type"] != "listItem" {
				t.Errorf("expected listItem, got %v", itemMap["type"])
			}
		}
	})

	t.Run("ordered list", func(t *testing.T) {
		markdown := "1. first\n2. second\n3. third"
		got, err := markdownToTiptap(markdown, nil)
		if err != nil {
			t.Fatalf("markdownToTiptap() error = %v", err)
		}

		content := got["content"].([]interface{})
		list := content[0].(map[string]interface{})

		if list["type"] != "orderedList" {
			t.Errorf("expected orderedList, got %v", list["type"])
		}

		listContent := list["content"].([]interface{})
		if len(listContent) != 3 {
			t.Errorf("expected 3 list items, got %d", len(listContent))
		}
	})
}

func TestMarkdownToTiptapImage(t *testing.T) {
	markdown := "![alt text](https://example.com/image.png)"
	got, err := markdownToTiptap(markdown, nil)
	if err != nil {
		t.Fatalf("markdownToTiptap() error = %v", err)
	}

	content := got["content"].([]interface{})
	para := content[0].(map[string]interface{})
	paraContent := para["content"].([]interface{})

	// Find image node
	found := false
	for _, node := range paraContent {
		nodeMap := node.(map[string]interface{})
		if nodeMap["type"] == "image" {
			attrs := nodeMap["attrs"].(map[string]interface{})
			if attrs["src"] == "https://example.com/image.png" && attrs["alt"] == "alt text" {
				found = true
			}
		}
	}

	if !found {
		t.Errorf("expected image node with src https://example.com/image.png not found")
	}
}

func TestMarkdownToTiptapJSON(t *testing.T) {
	// Verify that output is valid JSON
	markdown := "# Test\n\nHello **world**\n\n- item 1\n- item 2"
	got, err := markdownToTiptap(markdown, nil)
	if err != nil {
		t.Fatalf("markdownToTiptap() error = %v", err)
	}

	jsonBytes, err := json.Marshal(got)
	if err != nil {
		t.Errorf("markdownToTiptap() output is not valid JSON: %v", err)
	}

	// Verify we can unmarshal it back
	var unmarshaled map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &unmarshaled); err != nil {
		t.Errorf("markdownToTiptap() output cannot be unmarshaled: %v", err)
	}
}

func TestMarkdownToTiptapEmptyInput(t *testing.T) {
	got, err := markdownToTiptap("", nil)
	if err != nil {
		t.Fatalf("markdownToTiptap() error = %v", err)
	}

	if got["type"] != "doc" {
		t.Errorf("expected doc type for empty input")
	}

	content := got["content"].([]interface{})
	if len(content) != 0 {
		t.Errorf("expected empty content for empty input, got %d elements", len(content))
	}
}

func TestResolveManifestPaths(t *testing.T) {
	manifest := map[string]string{
		"diagram.png":    "https://cdn.andamio.io/images/abc/diagram.png",
		"screenshot.png": "https://cdn.andamio.io/images/abc/screenshot.png",
	}

	tests := []struct {
		name string
		md   string
		want string
	}{
		{
			name: "replaces local asset path",
			md:   "![A diagram](assets/diagram.png)",
			want: "![A diagram](https://cdn.andamio.io/images/abc/diagram.png)",
		},
		{
			name: "leaves external URLs unchanged",
			md:   "![img](https://example.com/img.png)",
			want: "![img](https://example.com/img.png)",
		},
		{
			name: "leaves unknown assets unchanged",
			md:   "![new](assets/new-image.png)",
			want: "![new](assets/new-image.png)",
		},
		{
			name: "replaces multiple occurrences",
			md:   "![a](assets/diagram.png)\n\n![b](assets/screenshot.png)",
			want: "![a](https://cdn.andamio.io/images/abc/diagram.png)\n\n![b](https://cdn.andamio.io/images/abc/screenshot.png)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveManifestPaths(tt.md, manifest)
			if got != tt.want {
				t.Errorf("resolveManifestPaths() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveManifestPathsNilManifest(t *testing.T) {
	md := "![img](assets/img.png)"
	got := resolveManifestPaths(md, nil)
	if got != md {
		t.Errorf("resolveManifestPaths() with nil manifest changed input: %q", got)
	}
}

func TestMarkdownToTiptapImageWithManifest(t *testing.T) {
	manifest := map[string]string{
		"diagram.png": "https://cdn.andamio.io/images/abc/diagram.png",
	}

	markdown := "![A diagram](assets/diagram.png)"
	got, err := markdownToTiptap(markdown, manifest)
	if err != nil {
		t.Fatalf("markdownToTiptap() error = %v", err)
	}

	content := got["content"].([]interface{})
	para := content[0].(map[string]interface{})
	paraContent := para["content"].([]interface{})

	found := false
	for _, node := range paraContent {
		nodeMap := node.(map[string]interface{})
		if nodeMap["type"] == "image" {
			attrs := nodeMap["attrs"].(map[string]interface{})
			if attrs["src"] == "https://cdn.andamio.io/images/abc/diagram.png" {
				found = true
			}
		}
	}

	if !found {
		t.Errorf("expected image with resolved CDN URL, got %v", content)
	}
}

func TestMarkdownToTiptapImageWithoutManifest(t *testing.T) {
	// Local image with no manifest should produce placeholder text
	markdown := "![A diagram](assets/diagram.png)"
	got, err := markdownToTiptap(markdown, nil)
	if err != nil {
		t.Fatalf("markdownToTiptap() error = %v", err)
	}

	content := got["content"].([]interface{})
	para := content[0].(map[string]interface{})
	paraContent := para["content"].([]interface{})

	found := false
	for _, node := range paraContent {
		nodeMap := node.(map[string]interface{})
		if text, ok := nodeMap["text"].(string); ok && text == "[Image: A diagram]" {
			found = true
		}
	}

	if !found {
		t.Errorf("expected placeholder text for unresolved image, got %v", content)
	}
}

func TestLoadImageManifest(t *testing.T) {
	// Non-existent directory returns empty map
	manifest := loadImageManifest("/nonexistent/path")
	if len(manifest) != 0 {
		t.Errorf("expected empty manifest for nonexistent path, got %d entries", len(manifest))
	}

	// Write a manifest to a temp dir and read it back
	dir := t.TempDir()
	manifestData := []byte(`{"diagram.png": "https://cdn.example.com/diagram.png"}`)
	manifestPath := dir + "/.image-manifest.json"
	if err := os.WriteFile(manifestPath, manifestData, 0644); err != nil {
		t.Fatalf("failed to write test manifest: %v", err)
	}

	manifest = loadImageManifest(dir)
	if len(manifest) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(manifest))
	}
	if manifest["diagram.png"] != "https://cdn.example.com/diagram.png" {
		t.Errorf("unexpected URL: %s", manifest["diagram.png"])
	}
}

func TestLoadImageManifestInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	manifestPath := dir + "/.image-manifest.json"
	if err := os.WriteFile(manifestPath, []byte("not json"), 0644); err != nil {
		t.Fatalf("failed to write test manifest: %v", err)
	}

	manifest := loadImageManifest(dir)
	if len(manifest) != 0 {
		t.Errorf("expected empty manifest for invalid JSON, got %d entries", len(manifest))
	}
}

func TestLoadImageManifestURLValidation(t *testing.T) {
	dir := t.TempDir()
	manifestData := []byte(`{
		"good.png": "https://cdn.example.com/good.png",
		"evil.png": "javascript:alert(1)",
		"data.png": "data:image/png;base64,abc",
		"http.png": "http://cdn.example.com/http.png"
	}`)
	if err := os.WriteFile(dir+"/.image-manifest.json", manifestData, 0644); err != nil {
		t.Fatalf("failed to write manifest: %v", err)
	}

	manifest := loadImageManifest(dir)

	// Only http/https URLs should be accepted
	if _, ok := manifest["good.png"]; !ok {
		t.Error("expected https URL to be accepted")
	}
	if _, ok := manifest["http.png"]; !ok {
		t.Error("expected http URL to be accepted")
	}
	if _, ok := manifest["evil.png"]; ok {
		t.Error("expected javascript: URL to be rejected")
	}
	if _, ok := manifest["data.png"]; ok {
		t.Error("expected data: URL to be rejected")
	}
}

func TestReadCompiledModuleWithManifest(t *testing.T) {
	// Integration test: create a full compiled module directory with manifest
	// and verify that readCompiledModule resolves image URLs correctly
	dir := t.TempDir()

	// Write outline.md
	outline := "---\ntitle: Test Module\ncode: \"101\"\n---\n\n# Test Module\n\n## SLTs\n\n1. Learn about images\n"
	if err := os.WriteFile(dir+"/outline.md", []byte(outline), 0644); err != nil {
		t.Fatal(err)
	}

	// Write lesson with local image reference
	lesson := "# Lesson 1\n\nHere is an image:\n\n![A diagram](assets/diagram.png)\n"
	if err := os.WriteFile(dir+"/lesson-1.md", []byte(lesson), 0644); err != nil {
		t.Fatal(err)
	}

	// Write manifest
	if err := os.MkdirAll(dir+"/assets", 0755); err != nil {
		t.Fatal(err)
	}
	manifest := `{"diagram.png": "https://cdn.andamio.io/images/abc/diagram.png"}`
	if err := os.WriteFile(dir+"/assets/.image-manifest.json", []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}

	data, err := readCompiledModule(dir)
	if err != nil {
		t.Fatalf("readCompiledModule() error = %v", err)
	}

	// Verify the lesson's Tiptap JSON contains the resolved CDN URL, not a placeholder
	if len(data.Lessons) != 1 {
		t.Fatalf("expected 1 lesson, got %d", len(data.Lessons))
	}

	lessonJSON, err := json.Marshal(data.Lessons[0].TiptapJSON)
	if err != nil {
		t.Fatal(err)
	}
	lessonStr := string(lessonJSON)

	if !strings.Contains(lessonStr, "https://cdn.andamio.io/images/abc/diagram.png") {
		t.Errorf("expected resolved CDN URL in lesson Tiptap JSON, got: %s", lessonStr)
	}
	if strings.Contains(lessonStr, "[Image:") {
		t.Errorf("found placeholder text instead of resolved image, got: %s", lessonStr)
	}

	// Verify no image warnings (diagram.png is in manifest)
	if len(data.ImageWarnings) != 0 {
		t.Errorf("expected no image warnings, got: %v", data.ImageWarnings)
	}
}
