---
title: "CLI Course Import Command with Markdown-to-Tiptap Conversion"
category: feature-implementations
tags: [goldmark, tiptap, markdown, ast, conversion, course-import, cli]
module: course-import
symptom: "Need to upload locally-edited course modules back to the Andamio platform"
root_cause: "Markdown files must be converted to Tiptap JSON format for the API"
---

# CLI Course Import Command with Markdown-to-Tiptap Conversion

## Overview

Implementation of `andamio course import <path> --course-id <id>` command that converts locally-edited Markdown course modules back to Tiptap JSON format for API upload. This is the inverse operation of `course export`.

## Solution

The command reads a compiled/ directory structure (outline.md with YAML frontmatter, lesson-N.md files), converts Markdown to Tiptap JSON using goldmark AST walker, and posts to the API endpoint to update the course module.

**Key components:**
- `readCompiledModule()` - reads directory structure and parses files
- `markdownToTiptap()` - converts Markdown to Tiptap JSON via AST walking
- `updateModuleContent()` - posts converted content to API

## Implementation

### 1. Goldmark Parser Setup with GFM Extensions

```go
import (
    "github.com/yuin/goldmark"
    "github.com/yuin/goldmark/ast"
    "github.com/yuin/goldmark/extension"
    extast "github.com/yuin/goldmark/extension/ast"  // Required for Strikethrough
    "github.com/yuin/goldmark/parser"
    "github.com/yuin/goldmark/text"
)

gm := goldmark.New(
    goldmark.WithExtensions(extension.GFM),
    goldmark.WithParserOptions(
        parser.WithAutoHeadingID(),
    ),
)

reader := text.NewReader([]byte(md))
doc := gm.Parser().Parse(reader)
```

**Critical:** GFM extension must be enabled for strikethrough support.

### 2. AST Node Type Switching

```go
func convertNode(n ast.Node, source []byte) interface{} {
    switch node := n.(type) {
    case *ast.Paragraph:
        return map[string]interface{}{
            "type":    "paragraph",
            "content": convertInlineContent(node, source),
        }

    case *ast.Heading:
        return map[string]interface{}{
            "type": "heading",
            "attrs": map[string]interface{}{
                "level": node.Level,
            },
            "content": convertInlineContent(node, source),
        }

    case *ast.List:
        listType := "bulletList"
        if node.IsOrdered() {
            listType = "orderedList"
        }
        // ... convert child list items
    }
}
```

### 3. Inline Marks Application

```go
case *ast.Emphasis:
    var children []interface{}
    for child := node.FirstChild(); child != nil; child = child.NextSibling() {
        nodes := convertInlineNode(child, source)
        children = append(children, nodes...)
    }

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
```

### 4. Strikethrough via Extension AST

```go
import extast "github.com/yuin/goldmark/extension/ast"

case *extast.Strikethrough:
    var children []interface{}
    for child := node.FirstChild(); child != nil; child = child.NextSibling() {
        nodes := convertInlineNode(child, source)
        children = append(children, nodes...)
    }

    for _, c := range children {
        if m, ok := c.(map[string]interface{}); ok {
            marks, _ := m["marks"].([]interface{})
            marks = append(marks, map[string]interface{}{"type": "strike"})
            m["marks"] = marks
        }
    }
    return children
```

**Critical:** Strikethrough is in `extension/ast` package, not main `ast` package.

### 5. Image Handling with URL Detection

```go
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

    // Local image → placeholder text (can't upload yet)
    return []interface{}{
        map[string]interface{}{
            "type": "text",
            "text": fmt.Sprintf("[Image: %s]", alt),
        },
    }
```

### Node Mapping Table

| Markdown | Tiptap Node |
|----------|-------------|
| Paragraph | `{"type": "paragraph", "content": [...]}` |
| `# Heading` | `{"type": "heading", "attrs": {"level": 1}, "content": [...]}` |
| `- item` | `{"type": "bulletList", "content": [{"type": "listItem", ...}]}` |
| `1. item` | `{"type": "orderedList", "content": [{"type": "listItem", ...}]}` |
| `` `code` `` | Text with `{"type": "code"}` mark |
| `**bold**` | Text with `{"type": "bold"}` mark |
| `*italic*` | Text with `{"type": "italic"}` mark |
| `[text](url)` | Text with `{"type": "link", "attrs": {"href": "..."}}` mark |
| `~~strike~~` | Text with `{"type": "strike"}` mark |
| ``` ```code``` ``` | `{"type": "codeBlock", "attrs": {"language": "..."}}` |
| `> quote` | `{"type": "blockquote", "content": [...]}` |
| `---` | `{"type": "horizontalRule"}` |

## Key Insights

### 1. Source Bytes Must Be Passed Through Entire Tree Walk

Goldmark doesn't store text in AST nodes—it stores byte ranges (segments). Every recursive call must pass the original `[]byte(md)`:

```go
text := string(node.Segment.Value(source))
```

### 2. Marks Are Applied Post-Hoc

Collect child content first, then mutate the `marks` array on text nodes:

```go
children := convertInlineNode(child, source)
for _, c := range children {
    m["marks"] = append(m["marks"], mark)
}
```

### 3. Block vs Inline Converters Have Different Signatures

- Block converters: return `map[string]interface{}` (single node)
- Inline converters: return `[]interface{}` (multiple nodes, marks applied)

### 4. Lesson File Ordering Requires Numeric Sort

Lexicographic sort fails: `lesson-1.md`, `lesson-10.md`, `lesson-2.md`. Extract and sort by number:

```go
re := regexp.MustCompile(`lesson-(\d+)\.md`)
num, _ := strconv.Atoi(matches[1])
```

## Prevention Strategies

### Before Writing Goldmark Code

1. **Enable required extensions** - GFM for strikethrough, tables, task lists
2. **Import extension AST package** - `extast "github.com/yuin/goldmark/extension/ast"`
3. **Plan block vs inline separation** - Different return types, different handling

### Common Mistakes

| Mistake | Fix |
|---------|-----|
| Missing strikethrough support | Import `extast`, add case for `*extast.Strikethrough` |
| Images silently fail | Check URL prefix, handle local vs external |
| Wrong lesson order | Extract number from filename, sort numerically |
| Text extraction fails | Pass `source []byte` through all calls |

## Testing Recommendations

### Unit Tests (11 cases implemented)

```go
func TestMarkdownToTiptap(t *testing.T) {
    tests := []struct {
        name     string
        markdown string
        wantType string
        wantLen  int
    }{
        {"simple paragraph", "Hello world", "doc", 1},
        {"heading level 1", "# Heading", "doc", 1},
        {"bullet list", "- item 1\n- item 2", "doc", 1},
        {"code block", "```bash\necho hello\n```", "doc", 1},
        // ...
    }
}
```

### Mark-Specific Tests

```go
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
}
```

## Related Documentation

- **Export (inverse operation):** [cli-course-export-tiptap-conversion.md](cli-course-export-tiptap-conversion.md)
- **API response parsing:** [../integration-issues/cli-export-api-response-structure-mismatch.md](../integration-issues/cli-export-api-response-structure-mismatch.md)
- **Security patterns:** [../security-issues/cli-security-hardening-input-validation.md](../security-issues/cli-security-hardening-input-validation.md)
- **Auth integration:** [../integration-issues/cli-api-auth-middleware-mismatch.md](../integration-issues/cli-api-auth-middleware-mismatch.md)

## Files

- Implementation: `cmd/andamio/course_import.go`
- Tests: `cmd/andamio/course_import_test.go`
- Plan: `docs/plans/2026-03-16-feat-course-import-text-only-plan.md`
