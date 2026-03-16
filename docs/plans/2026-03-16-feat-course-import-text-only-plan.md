---
title: "feat: Course Module Import (Text Only)"
type: feat
status: active
date: 2026-03-16
origin: docs/plans/2026-03-16-feat-course-module-import-export-plan.md
---

# Course Module Import (Text Only)

## Overview

Implement `andamio course import <path> --course-id <id>` to import locally-edited course modules back to the platform. This initial version handles **text content only** - image upload will be added once the CDN endpoint is discovered.

## Scope

**In scope:**
- Parse `compiled/` directory structure
- Convert Markdown → Tiptap JSON
- Update module content via API
- Warn if local images exist (can't upload yet)

**Out of scope (future):**
- Image upload to CDN
- Creating new modules (only updating existing)

## Technical Approach

### Dependencies to Add

```bash
go get github.com/yuin/goldmark
go get github.com/adrg/frontmatter
```

### Implementation

#### 1. Command Structure

```go
// cmd/andamio/course_import.go

var courseImportCmd = &cobra.Command{
    Use:   "import <path>",
    Short: "Import a compiled module to update course content",
    Long: `Import a compiled module directory to update an existing course module.

The directory should contain:
  - outline.md (with YAML frontmatter: title, code)
  - lesson-N.md files
  - introduction.md (optional)
  - assignment.md (optional)

Note: Image upload is not yet supported. Images in assets/ will be skipped.`,
    Args: cobra.ExactArgs(1),
    PreRunE: requireUserAuth,
}
```

#### 2. Read Compiled Module

```go
type ImportData struct {
    Title        string
    ModuleCode   string
    SLTs         []string           // From outline.md
    Lessons      []LessonImport     // lesson-1.md, lesson-2.md, etc.
    Introduction *string            // Optional
    Assignment   *string            // Optional
    ImageWarnings []string          // Images that couldn't be uploaded
}

type LessonImport struct {
    Index      int
    Markdown   string
    TiptapJSON map[string]interface{}
}

func readCompiledModule(dir string) (*ImportData, error) {
    // 1. Read and parse outline.md (YAML frontmatter)
    // 2. Read lesson-N.md files in order
    // 3. Read introduction.md if exists
    // 4. Read assignment.md if exists
    // 5. Check assets/ for images and warn
}
```

#### 3. Markdown → Tiptap Conversion

Using goldmark with a custom renderer:

```go
import (
    "github.com/yuin/goldmark"
    "github.com/yuin/goldmark/ast"
    "github.com/yuin/goldmark/renderer"
)

func markdownToTiptap(md string) (map[string]interface{}, error) {
    // Parse markdown to AST
    parser := goldmark.DefaultParser()
    reader := text.NewReader([]byte(md))
    doc := parser.Parse(reader)

    // Walk AST and build Tiptap JSON
    result := map[string]interface{}{
        "type": "doc",
        "content": []interface{}{},
    }

    // Convert each node
    ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
        if !entering {
            return ast.WalkContinue, nil
        }
        // Handle: Paragraph, Heading, List, ListItem, CodeBlock, Blockquote, etc.
    })

    return result, nil
}
```

#### 4. API Update Call

```go
func updateModule(c *client.Client, courseID string, data *ImportData) error {
    // Build request payload matching API schema
    payload := map[string]interface{}{
        "course_id": courseID,
        "course_module_code": data.ModuleCode,
        "title": data.Title,
        "slts": buildSLTsPayload(data),
        "lessons": buildLessonsPayload(data),
    }

    if data.Introduction != nil {
        payload["introduction"] = markdownToTiptap(*data.Introduction)
    }
    if data.Assignment != nil {
        payload["assignment"] = markdownToTiptap(*data.Assignment)
    }

    var resp map[string]interface{}
    return c.Post("/api/v2/course/teacher/course-module/update", payload, &resp)
}
```

### Node Mapping (Markdown → Tiptap)

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
| `![alt](src)` | **WARN** - can't upload, skip with warning |

### Error Handling

| Error | Response |
|-------|----------|
| Directory doesn't exist | "Directory not found: {path}" |
| Missing outline.md | "Missing outline.md in {path}" |
| Invalid YAML frontmatter | "Invalid outline.md: {details}" |
| SLT count mismatch | "Found {n} lessons but outline lists {m} SLTs" |
| Module doesn't exist | "Module {code} not found in course. Use web UI to create first." |
| API auth failure | "Not authenticated or no teacher access" |
| Images found | "Warning: {n} images in assets/ skipped (upload not yet supported)" |

## Acceptance Criteria

- [x] `andamio course import <path> --course-id <id>` updates module content
- [x] Parses outline.md YAML frontmatter correctly
- [x] Converts all supported Markdown constructs to Tiptap
- [x] Warns about images that can't be uploaded
- [x] Clear error messages for all failure modes
- [x] Unit tests for markdownToTiptap conversion

## Tasks

- [x] Add goldmark and frontmatter dependencies
- [x] Create `cmd/andamio/course_import.go` with command structure
- [x] Implement `readCompiledModule()`
- [x] Implement `markdownToTiptap()` using goldmark AST walker
- [x] Implement `updateModule()` API call
- [x] Add unit tests for Markdown → Tiptap conversion
- [x] Test with exported module (roundtrip)
- [ ] Update README

## Future: Image Upload

Once CDN endpoint is discovered, add:
1. Read images from assets/
2. Validate magic bytes (PNG/JPEG only)
3. Upload to CDN, get hosted URL
4. Replace `![](assets/file.png)` with `![](https://cdn.../file.png)` in Tiptap output

## Sources

- **Origin plan:** [docs/plans/2026-03-16-feat-course-module-import-export-plan.md](2026-03-16-feat-course-module-import-export-plan.md)
- **Export implementation:** `cmd/andamio/course_export.go`
- **goldmark docs:** https://github.com/yuin/goldmark
- **API endpoint:** `POST /v2/course/teacher/course-module/update`
