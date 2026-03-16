---
title: "Goldmark AST Walker Prevention Strategies"
date: 2026-03-16
category: architecture
tags:
  - ast-conversion
  - markdown
  - tiptap
  - goldmark
  - best-practices
  - prevention-strategies
components:
  - cmd/andamio/course_import.go
  - cmd/andamio/course_import_test.go
---

# Goldmark AST Walker Prevention Strategies

## Context

Implemented Markdown → Tiptap JSON conversion using goldmark AST walker for the course import feature. This document captures lessons learned and prevention strategies for future similar work.

## Challenges Encountered

### 1. Strikethrough in Extension Package

**Problem:** Strikethrough nodes are in `github.com/yuin/goldmark/extension/ast`, not the main `ast` package.

**Initial Mistake:**
```go
// WRONG - Strikethrough not found in ast package
case *ast.Strikethrough:
    // panic: type not found
```

**Solution:**
```go
// RIGHT - Must import extension/ast separately
import (
    "github.com/yuin/goldmark/ast"
    extast "github.com/yuin/goldmark/extension/ast"
)

case *extast.Strikethrough:
    // Apply strike mark
```

**Prevention:** Always check the goldmark documentation to identify which extension provides each node type. GFM-specific nodes (strikethrough, tables, footnotes) live in extension packages.

---

### 2. GFM Extension Must Be Enabled

**Problem:** Strikethrough and other GFM features silently don't parse if extension is not enabled.

**Initial Mistake:**
```go
// WRONG - GFM extension not loaded
gm := goldmark.New()
```

**Solution:**
```go
// RIGHT - Enable GFM extension explicitly
import "github.com/yuin/goldmark/extension"

gm := goldmark.New(
    goldmark.WithExtensions(extension.GFM),
    goldmark.WithParserOptions(
        parser.WithAutoHeadingID(),
    ),
)
```

**Prevention:** When parsing Markdown with extended syntax (strikethrough, tables, task lists), explicitly load the GFM extension. Test with sample markdown to verify parsing works before converting.

---

### 3. Image URL Handling Requires Type Differentiation

**Problem:** Images in different contexts (inline vs block) need different JSON representations. External URLs vs local assets need different handling.

**Context 1: Block-level images in paragraphs**
```go
// Images can appear as inline content
case *ast.Image:
    src := string(node.Destination)
    alt := string(node.Text(source))

    // External URLs → proper image node
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

    // Local assets → placeholder text (can't upload)
    return []interface{}{
        map[string]interface{}{
            "type": "text",
            "text": fmt.Sprintf("[Image: %s]", alt),
        },
    }
```

**Context 2: Image nodes at block level**
```go
// Same node type, different handling at block level
case *ast.Image:
    alt := string(node.Text(source))
    if alt == "" {
        alt = "[image]"
    }
    // Return paragraph wrapper, not bare image node
    return map[string]interface{}{
        "type": "paragraph",
        "content": []interface{}{
            map[string]interface{}{
                "type": "text",
                "text": fmt.Sprintf("[Image: %s]", alt),
            },
        },
    }
```

**Prevention:**
- Test images in both inline and block contexts
- Handle external URLs (http/https) differently from local paths
- Document which image types your target JSON format supports
- Add image URL validation during conversion, not after

---

### 4. Inline vs Block Node Conversion Requires Different Approaches

**Problem:** Inline nodes return `[]interface{}` (can be multiple marks on same text), block nodes return `map[string]interface{}` (single node structure).

**Block-level conversion pattern:**
```go
func convertSingleNode(n ast.Node, source []byte) map[string]interface{} {
    // Returns single node map or nil
    switch node := n.(type) {
    case *ast.Paragraph:
        return map[string]interface{}{
            "type":    "paragraph",
            "content": convertInlineContent(node, source),
        }
    case *ast.Heading:
        return map[string]interface{}{
            "type": "heading",
            "attrs": map[string]interface{}{"level": node.Level},
            "content": convertInlineContent(node, source),
        }
    default:
        return nil
    }
}

func convertNode(n ast.Node, source []byte) []interface{} {
    // Wraps single nodes into array
    var result []interface{}
    for child := n.FirstChild(); child != nil; child = child.NextSibling() {
        node := convertSingleNode(child, source)
        if node != nil {
            result = append(result, node)
        }
    }
    return result
}
```

**Inline-level conversion pattern:**
```go
func convertInlineNode(n ast.Node, source []byte) []interface{} {
    // Returns array - multiple nodes or marks on same text
    switch node := n.(type) {
    case *ast.Text:
        return []interface{}{
            map[string]interface{}{
                "type": "text",
                "text": string(node.Segment.Value(source)),
            },
        }

    case *ast.Emphasis:
        // Recursively process children and apply marks
        var children []interface{}
        for child := node.FirstChild(); child != nil; child = child.NextSibling() {
            nodes := convertInlineNode(child, source)
            children = append(children, nodes...)
        }

        // Apply emphasis mark to all text nodes
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
        // Same pattern as emphasis
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

    default:
        return nil
    }
}
```

**Prevention:**
- Keep block and inline conversion functions separate
- Block converters: `func convertSingleNode(...) map[string]interface{}`
- Inline converters: `func convertInlineNode(...) []interface{}`
- Test nested formatting (bold + italic, bold + strikethrough) to verify mark accumulation

---

## Prevention Strategies for Future Goldmark Work

### 1. **Initialize Extensions Correctly**

**Checklist:**
- [ ] Identify all markdown features you need to parse (strikethrough, tables, task lists, etc.)
- [ ] Check goldmark documentation for which extension provides each feature
- [ ] Load extensions in goldmark.New():
  ```go
  gm := goldmark.New(
      goldmark.WithExtensions(
          extension.GFM,          // For strikethrough, tables
          extension.Footnote,     // For footnotes if needed
      ),
      goldmark.WithParserOptions(
          parser.WithAutoHeadingID(),
      ),
  )
  ```
- [ ] Test with actual markdown samples containing each feature

**Common Extensions:**
- `extension.GFM` - GitHub Flavored Markdown (strikethrough, tables, task lists, autolinks)
- `extension.Footnote` - Footnotes with [^1] syntax
- `extension.Typographer` - Smart quotes, em-dashes
- `extension.CJK` - Better CJK text handling
- `extension.Linkify` - Automatic URL linking

### 2. **Import Node Types from Correct Packages**

**Pattern:**
```go
import (
    "github.com/yuin/goldmark/ast"
    extast "github.com/yuin/goldmark/extension/ast"  // GFM-specific nodes
)
```

**Node Location Quick Reference:**
| Node Type | Package | Notes |
|-----------|---------|-------|
| Paragraph, Heading, List | `ast` | Core markdown |
| Emphasis, Strong, Code | `ast` | Inline formatting |
| Link, Image, AutoLink | `ast` | References |
| FencedCodeBlock, CodeBlock | `ast` | Code blocks |
| Blockquote, Thematic Break | `ast` | Block elements |
| Strikethrough | `extension/ast` | Requires GFM |
| Table, TableCell | `extension/ast` | Requires GFM |
| TaskListItem | `extension/ast` | Requires GFM |
| Footnote | `extension/ast` | Requires extension |

### 3. **Separate Block and Inline Conversion**

**Structural Pattern:**

```go
// Block-level: processes all children, returns array
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

// Single block node conversion
func convertSingleNode(n ast.Node, source []byte) map[string]interface{} {
    switch node := n.(type) {
    case *ast.Paragraph:
        // Block-level: wraps inline content
        content := convertInlineContent(node, source)
        if len(content) == 0 {
            return nil
        }
        return map[string]interface{}{
            "type":    "paragraph",
            "content": content,
        }
    default:
        return nil
    }
}

// Inline content wrapper (collects all inline nodes)
func convertInlineContent(n ast.Node, source []byte) []interface{} {
    var result []interface{}
    for child := n.FirstChild(); child != nil; child = child.NextSibling() {
        nodes := convertInlineNode(child, source)
        result = append(result, nodes...)  // Flatten array
    }
    return result
}

// Inline node conversion (returns array of nodes/marked text)
func convertInlineNode(n ast.Node, source []byte) []interface{} {
    switch node := n.(type) {
    case *ast.Text:
        return []interface{}{
            map[string]interface{}{
                "type": "text",
                "text": string(node.Segment.Value(source)),
            },
        }
    case *ast.Emphasis:
        // Process children and apply marks
        var children []interface{}
        for child := node.FirstChild(); child != nil; child = child.NextSibling() {
            nodes := convertInlineNode(child, source)
            children = append(children, nodes...)
        }

        // Apply marks to each text node
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
    default:
        return nil
    }
}
```

**Why This Works:**
- Block converters have a single responsibility (one node → one output)
- Inline converters handle mark stacking naturally (text node can have multiple marks)
- Separation makes testing easier (mock data easier to construct)
- Clear contract: block returns map, inline returns array

### 4. **Handle Image Types Explicitly**

**Strategy:**

```go
// Decide during conversion time, not after
case *ast.Image:
    src := string(node.Destination)
    alt := string(node.Text(source))

    // Validate URL
    if src == "" {
        return nil  // Skip malformed images
    }

    // Categorize and handle
    if isExternalURL(src) {
        // External: create proper image node
        return []interface{}{
            map[string]interface{}{
                "type": "image",
                "attrs": map[string]interface{}{
                    "src": src,
                    "alt": alt,
                },
            },
        }
    } else if isLocalAsset(src) {
        // Local: create placeholder (can't upload)
        return []interface{}{
            map[string]interface{}{
                "type": "text",
                "text": fmt.Sprintf("[Image: %s]", alt),
            },
        }
    } else {
        // Unknown: warn and skip
        return nil
    }

func isExternalURL(s string) bool {
    return strings.HasPrefix(s, "http://") ||
           strings.HasPrefix(s, "https://")
}

func isLocalAsset(s string) bool {
    return !strings.HasPrefix(s, "http") &&
           (strings.HasPrefix(s, "./assets/") ||
            strings.HasPrefix(s, "assets/"))
}
```

**Testing Images:**
- Test external URLs: `![alt](https://example.com/image.png)`
- Test local assets: `![alt](./assets/image.png)`
- Test relative paths: `![alt](assets/image.png)`
- Test missing alt: `![](https://example.com/image.png)`
- Test missing src: `![alt]()`

### 5. **Validate and Extract Text Correctly**

**Patterns for Text Extraction:**

```go
// From Text node (segment-based)
case *ast.Text:
    text := string(node.Segment.Value(source))
    // Important: Segment.Value() extracts the actual bytes

// From CodeSpan (inline code)
case *ast.CodeSpan:
    text := string(node.Text(source))
    // CodeSpan has Text() method

// From Image/Link (references)
case *ast.Image:
    alt := string(node.Text(source))
    src := string(node.Destination)

// From Lines (block code)
case *ast.FencedCodeBlock:
    var code strings.Builder
    lines := node.Lines()
    for i := 0; i < lines.Len(); i++ {
        line := lines.At(i)
        code.Write(line.Value(source))
    }
    // Always write trailing bytes carefully
    text := strings.TrimSuffix(code.String(), "\n")
```

**Prevention:**
- Always pass the original `source []byte` to node extraction methods
- Use `node.Text(source)` for most nodes, `node.Segment.Value(source)` for Text nodes
- For multi-line content (code blocks), iterate Lines() and use `line.Value(source)`
- Trim trailing newlines from code blocks

### 6. **Create Comprehensive Node Type Registry**

**Document Your Conversions:**

```go
// Supported node types and their conversion patterns
// Block Nodes (returns single map or nil)
// - Paragraph → {type: "paragraph", content: [...inline nodes]}
// - Heading → {type: "heading", attrs: {level: N}, content: [...]}
// - List → {type: "bulletList"|"orderedList", content: [...items]}
// - ListItem → {type: "listItem", content: [...block nodes]}
// - BlockQuote → {type: "blockquote", content: [...]}
// - CodeBlock/FencedCodeBlock → {type: "codeBlock", attrs: {language}, content: [...]}
// - Image (block context) → {type: "paragraph", content: [...]} (with image placeholder)

// Inline Nodes (returns array of nodes)
// - Text → [{type: "text", text: "content"}]
// - Emphasis → [{type: "text", text: "...", marks: [{type: "italic"|"bold"}]}]
// - Strikethrough → [{type: "text", text: "...", marks: [{type: "strike"}]}]
// - CodeSpan → [{type: "text", text: "...", marks: [{type: "code"}]}]
// - Link → [{type: "text", text: "...", marks: [{type: "link", attrs: {href}}]}]
// - Image (inline) → [{type: "image", attrs: {src, alt}}]

var supportedNodes = []string{
    // Block
    "Paragraph", "Heading", "List", "ListItem",
    "BlockQuote", "FencedCodeBlock", "CodeBlock",
    "ThematicBreak", "Image",
    // Inline
    "Text", "Emphasis", "CodeSpan", "Link", "AutoLink", "Image",
    // GFM
    "Strikethrough", "Table", "TaskListItem",
}
```

### 7. **Test All Node Type Combinations**

**Test Matrix:**

```go
func TestMarkdownToTiptap(t *testing.T) {
    tests := []struct {
        name     string
        markdown string
        wantType string
        wantLen  int
        check    func(t *testing.T, got map[string]interface{})
    }{
        // Basic blocks
        {"simple paragraph", "Hello world", "doc", 1, nil},
        {"heading h1", "# Title", "doc", 1, checkHeadingLevel(1)},
        {"heading h2", "## Subtitle", "doc", 1, checkHeadingLevel(2)},

        // Inline formatting
        {"bold text", "**bold**", "doc", 1, checkMark("bold")},
        {"italic text", "*italic*", "doc", 1, checkMark("italic")},
        {"code text", "`code`", "doc", 1, checkMark("code")},
        {"strikethrough", "~~strike~~", "doc", 1, checkMark("strike")},

        // Combinations (crucial!)
        {"bold + italic", "***both***", "doc", 1, checkMarks("bold", "italic")},
        {"bold + strikethrough", "**~~both~~**", "doc", 1, checkMarks("bold", "strike")},
        {"link with formatting", "[**link**](url)", "doc", 1, checkMarks("bold", "link")},

        // Lists
        {"bullet list", "- item\n- item", "doc", 1, checkListType("bulletList")},
        {"ordered list", "1. first\n2. second", "doc", 1, checkListType("orderedList")},
        {"nested list", "- item\n  - nested", "doc", 1, checkNestedList()},

        // Code
        {"code block", "```\ncode\n```", "doc", 1, checkCodeBlock("")},
        {"code with lang", "```python\ncode\n```", "doc", 1, checkCodeBlock("python")},

        // Images
        {"external image", "![alt](https://example.com/img.png)", "doc", 1, checkImage()},
        {"local asset image", "![alt](./assets/img.png)", "doc", 1, checkImagePlaceholder()},

        // Edge cases
        {"empty input", "", "doc", 0, nil},
        {"only whitespace", "   ", "doc", 0, nil},
        {"multiple paragraphs", "p1\n\np2", "doc", 2, nil},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := markdownToTiptap(tt.markdown)
            if err != nil {
                t.Fatalf("error: %v", err)
            }

            if got["type"] != tt.wantType {
                t.Errorf("type = %v, want %v", got["type"], tt.wantType)
            }

            content := got["content"].([]interface{})
            if len(content) != tt.wantLen {
                t.Errorf("len = %d, want %d", len(content), tt.wantLen)
            }

            if tt.check != nil {
                tt.check(t, got)
            }
        })
    }
}
```

---

## Testing Recommendations

### 1. **Unit Tests for Node Conversion Functions**

Test each function in isolation with known inputs:

```go
func TestConvertInlineNode(t *testing.T) {
    tests := []struct {
        name     string
        markdown string
        wantNode string
        wantText string
    }{
        {"bold", "**bold**", "text", "bold"},
        {"italic", "*italic*", "text", "italic"},
        {"code", "`code`", "text", "code"},
        {"strikethrough", "~~strike~~", "text", "strike"},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            gm := createGoldmarkWithExtensions()
            doc := gm.Parser().Parse(text.NewReader([]byte(tt.markdown)))

            // Navigate to inline node
            para := doc.FirstChild().(*ast.Paragraph)
            node := para.FirstChild()

            result := convertInlineNode(node, []byte(tt.markdown))
            if len(result) == 0 {
                t.Fatal("expected result")
            }

            nodeMap := result[0].(map[string]interface{})
            if nodeMap["type"] != tt.wantNode {
                t.Errorf("type = %v, want %v", nodeMap["type"], tt.wantNode)
            }
        })
    }
}
```

### 2. **Integration Tests for Full Conversion**

Test end-to-end markdown → JSON:

```go
func TestMarkdownToTiptapIntegration(t *testing.T) {
    markdown := `# Title

This is **bold** and *italic* text.

- Item 1
- Item 2

\`\`\`go
fmt.Println("code")
\`\`\`
`

    got, err := markdownToTiptap(markdown)
    if err != nil {
        t.Fatalf("error: %v", err)
    }

    // Verify structure
    content := got["content"].([]interface{})
    if len(content) < 4 {
        t.Errorf("expected at least 4 elements, got %d", len(content))
    }

    // Verify heading
    heading := content[0].(map[string]interface{})
    if heading["type"] != "heading" {
        t.Error("expected heading")
    }

    // Verify list
    list := content[2].(map[string]interface{})
    if list["type"] != "bulletList" {
        t.Error("expected bulletList")
    }
}
```

### 3. **Round-Trip Tests for Import/Export**

Test that markdown survives conversion:

```go
func TestMarkdownRoundTrip(t *testing.T) {
    original := "# Title\n\n**bold** text\n\n- item 1\n- item 2"

    // Convert to Tiptap
    tiptap, err := markdownToTiptap(original)
    if err != nil {
        t.Fatalf("error: %v", err)
    }

    // Convert back to markdown
    recovered, _ := tiptapToMarkdown(tiptap)

    // Compare key features (not byte-for-byte, as formatting may differ)
    if !strings.Contains(recovered, "Title") {
        t.Error("lost heading")
    }
    if !strings.Contains(recovered, "bold") {
        t.Error("lost bold formatting")
    }
    if !strings.Contains(recovered, "item 1") {
        t.Error("lost list items")
    }
}
```

### 4. **Golden File Tests for Complex Documents**

Store expected output for complex markdown:

```go
func TestComplexDocument(t *testing.T) {
    // Read markdown file
    input, _ := os.ReadFile("testdata/complex.md")

    // Convert to Tiptap
    got, _ := markdownToTiptap(string(input))

    // Read expected output
    expected, _ := os.ReadFile("testdata/complex.tiptap.json")
    var want map[string]interface{}
    json.Unmarshal(expected, &want)

    // Compare (with custom equality for JSON)
    if !mapsEqual(got, want) {
        gotJSON, _ := json.MarshalIndent(got, "", "  ")
        t.Errorf("output mismatch:\n%s", gotJSON)
    }
}

// testdata/complex.md - Real-world markdown document
// testdata/complex.tiptap.json - Expected Tiptap output
```

### 5. **Error Handling Tests**

Test graceful failure modes:

```go
func TestConversionErrors(t *testing.T) {
    tests := []struct {
        name      string
        markdown  string
        shouldErr bool
    }{
        {"empty string", "", false},
        {"only whitespace", "   ", false},
        {"invalid utf8", "\xff\xfe", true},
        {"huge document", strings.Repeat("a", 10*1024*1024), true},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            _, err := markdownToTiptap(tt.markdown)
            if (err != nil) != tt.shouldErr {
                t.Errorf("error = %v, shouldErr = %v", err, tt.shouldErr)
            }
        })
    }
}
```

### 6. **Extension Coverage Tests**

Verify all enabled extensions are tested:

```go
func TestGFMExtensions(t *testing.T) {
    tests := []struct {
        name     string
        markdown string
        check    func(t *testing.T, got map[string]interface{})
    }{
        // Strikethrough (GFM)
        {"strikethrough", "~~crossed~~", func(t *testing.T, got map[string]interface{}) {
            content := got["content"].([]interface{})
            para := content[0].(map[string]interface{})
            paraContent := para["content"].([]interface{})

            found := false
            for _, node := range paraContent {
                nodeMap := node.(map[string]interface{})
                if marks, ok := nodeMap["marks"].([]interface{}); ok {
                    for _, mark := range marks {
                        if mark.(map[string]interface{})["type"] == "strike" {
                            found = true
                        }
                    }
                }
            }
            if !found {
                t.Error("strikethrough mark not found")
            }
        }},

        // TODO: Table tests (if implemented)
        // TODO: Task list tests (if implemented)
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := markdownToTiptap(tt.markdown)
            if err != nil {
                t.Fatalf("error: %v", err)
            }
            if tt.check != nil {
                tt.check(t, got)
            }
        })
    }
}
```

### 7. **Regression Test Suite**

Add a test for every bug found during development:

```go
func TestRegressions(t *testing.T) {
    // BUG #1: Strikethrough not parsed because GFM extension not enabled
    t.Run("strikethrough_requires_gfm", func(t *testing.T) {
        // Verify that strikethrough is parsed
        got, _ := markdownToTiptap("~~strike~~")
        // ... assertion
    })

    // BUG #2: Image URLs (external vs local) not differentiated
    t.Run("external_image_creates_image_node", func(t *testing.T) {
        // Verify external URLs create <image> nodes
    })

    t.Run("local_asset_creates_placeholder", func(t *testing.T) {
        // Verify local assets create text placeholders
    })

    // BUG #3: Nested formatting marks not accumulated
    t.Run("bold_and_italic_marks_accumulate", func(t *testing.T) {
        // Verify both marks present on text
    })
}
```

---

## Summary Checklist for Future Goldmark Work

### Before Writing Code

- [ ] Read goldmark documentation for your target features
- [ ] Identify which extensions you need (GFM, Footnote, etc.)
- [ ] Map all markdown features → goldmark node types
- [ ] Identify which nodes are in main `ast` package vs extensions
- [ ] Document your target output format's node types and marks
- [ ] Design test cases for all node combinations

### During Implementation

- [ ] Load extensions in goldmark.New()
- [ ] Import node types from correct packages (use aliases for clarity)
- [ ] Separate block and inline conversion logic
- [ ] Test with actual markdown samples for each feature
- [ ] Handle edge cases (empty content, missing attributes, etc.)
- [ ] Add type assertions with error handling

### After Implementation

- [ ] Write unit tests for each conversion function
- [ ] Write integration tests for full end-to-end conversion
- [ ] Write tests for all node type combinations
- [ ] Test edge cases and error conditions
- [ ] Create golden file tests for complex documents
- [ ] Add regression tests for bugs found during testing
- [ ] Verify all enabled extensions are actually tested
- [ ] Test round-trip conversion if applicable

---

## References

- **goldmark documentation:** https://github.com/yuin/goldmark
- **Implementation:** `/Users/james/projects/01-projects/andamio-cli/cmd/andamio/course_import.go`
- **Tests:** `/Users/james/projects/01-projects/andamio-cli/cmd/andamio/course_import_test.go`
- **Tiptap format:** https://www.tiptap.dev/guide/output#json
- **ProseMirror nodes:** https://prosemirror.net/docs/ref/#model

