---
title: "CLI Course Export Command with Tiptap-to-Markdown Conversion"
date: 2026-03-16
category: feature-implementations
tags:
  - cli-command
  - tiptap-conversion
  - concurrent-api
  - atomic-writes
  - security
  - markdown-export
  - go
components:
  - cmd/andamio/course_export.go
  - cmd/andamio/course_export_test.go
symptoms:
  - "Need to export course modules to editable markdown"
  - "Tiptap JSON format incompatible with local editing"
  - "Manual module export process is time-consuming"
root_cause: "CLI lacked command to convert rich text course content to standard markdown format compatible with lesson-coach workflow"
severity: low
time_to_resolve: "4 hours (research, implementation, testing)"
---

# CLI Course Export Command Implementation

## Problem

The Andamio platform stores course content in Tiptap JSON format (ProseMirror-based rich text), but Andamio Pioneers need to:

1. Export course modules for local editing in their preferred editors
2. Version control course materials in git
3. Round-trip edit: export → modify → re-import

The lesson-coach tool produces a `compiled/` directory structure, but there was no way to export existing platform content to this format.

## Solution

Implemented `andamio course export <course-id> <module-code>` command that:

1. Fetches module data via teacher API endpoints
2. Converts Tiptap JSON to Markdown
3. Downloads images to local assets/ directory
4. Writes files atomically to prevent corruption

### Key Implementation Patterns

#### 1. Tiptap to Markdown Conversion

Recursive converter handles all Tiptap node types:

```go
func tiptapToMarkdown(doc map[string]interface{}) (string, []string) {
    var sb strings.Builder
    var imageURLs []string

    content, _ := doc["content"].([]interface{})
    for _, node := range content {
        nodeMap, _ := node.(map[string]interface{})
        nodeType, _ := nodeMap["type"].(string)

        switch nodeType {
        case "paragraph":
            text := extractTextContent(nodeMap)
            sb.WriteString(text + "\n\n")
        case "heading":
            level := int(nodeMap["attrs"].(map[string]interface{})["level"].(float64))
            text := extractTextContent(nodeMap)
            sb.WriteString(strings.Repeat("#", level) + " " + text + "\n\n")
        case "bulletList":
            // Recursive list handling
        case "image":
            attrs := nodeMap["attrs"].(map[string]interface{})
            src := attrs["src"].(string)
            alt := attrs["alt"].(string)
            imageURLs = append(imageURLs, src)
            sb.WriteString(fmt.Sprintf("![%s](assets/%s)\n\n", alt, filename))
        // ... other node types
        }
    }
    return sb.String(), imageURLs
}
```

**Supported nodes:** doc, paragraph, heading, bulletList, orderedList, listItem, blockquote, codeBlock, image, hardBreak, horizontalRule

**Supported marks:** bold, italic, code, link, strike

#### 2. Parallel API Fetching

Semaphore pattern limits concurrent requests:

```go
sem := make(chan struct{}, 5) // 5 concurrent requests
var wg sync.WaitGroup
errChan := make(chan error, len(sltsData))

for _, slt := range sltsData {
    wg.Add(1)
    go func(slt SLTData) {
        defer wg.Done()
        sem <- struct{}{}        // Acquire slot
        defer func() { <-sem }() // Release slot

        // Fetch lesson from API
        if err := c.Get(path, &resp); err != nil {
            errChan <- err
            return
        }
        // Process response
    }(slt)
}

wg.Wait()
close(errChan)
```

#### 3. Atomic File Writes

Temp-then-rename prevents partial writes:

```go
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
    dir := filepath.Dir(path)
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

    if _, err := tmp.Write(data); err != nil {
        return err
    }
    if err := tmp.Sync(); err != nil {
        return err
    }
    if err := tmp.Close(); err != nil {
        return err
    }

    success = true
    return os.Rename(tmpPath, path)
}
```

#### 4. Security: Path Validation

Prevents directory traversal:

```go
absDir, err := filepath.Abs(outputDir)
if err != nil {
    return fmt.Errorf("invalid output directory: %w", err)
}

// URL validation for image downloads
parsed, err := url.Parse(imgURL)
if parsed.Scheme != "https" && parsed.Scheme != "http" {
    fmt.Printf("Warning: skipping non-HTTP URL: %s\n", imgURL)
    continue
}

// Size limits on downloads
data, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024)) // 10MB
```

### Bug Fixes During Implementation

1. **Flag shorthand conflict:** Changed `-o` to `--output-dir` (no shorthand) to avoid conflict with global `--output` flag

2. **fmt.Fprintf format string warning:** Changed `fmt.Fprintf(w, authFailureHTML(...))` to `fmt.Fprint(w, ...)` since HTML strings aren't format strings

## Output Structure

```
compiled/<course-slug>/<module-code>/
├── outline.md          # YAML frontmatter + SLT list
├── introduction.md     # Module introduction (if exists)
├── lesson-1.md         # Lesson for SLT 1
├── lesson-N.md         # Lesson for SLT N
├── assignment.md       # Module assignment (if exists)
└── assets/             # Downloaded images
    └── *.png
```

## Testing

Table-driven tests cover all transformation functions:

```go
func TestTiptapToMarkdown(t *testing.T) {
    tests := []struct {
        name     string
        input    map[string]interface{}
        wantMD   string
        wantURLs []string
    }{
        {"simple paragraph", paragraphDoc, "Hello world\n\n", nil},
        {"heading level 2", headingDoc, "## Title\n\n", nil},
        {"bold text", boldDoc, "**bold**\n\n", nil},
        // ... 16 test cases total
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            gotMD, gotURLs := tiptapToMarkdown(tt.input)
            if gotMD != tt.wantMD {
                t.Errorf("got %q, want %q", gotMD, tt.wantMD)
            }
        })
    }
}
```

## Prevention & Best Practices

### For Future CLI Commands

1. **Auth pattern:** Use `PreRunE` for auth checks (not `PersistentPreRunE`)
2. **Parallel ops:** Always use semaphore pattern with buffered error channel
3. **File I/O:** Use atomic writes for all file operations
4. **Security:** Validate paths with `filepath.Abs()`, URLs with scheme checks
5. **Flags:** Avoid shorthand conflicts with global `-o` flag
6. **Testing:** Table-driven tests for content transformations

### Command Checklist

- [ ] Auth check in PreRunE if needed
- [ ] Semaphore for concurrent API calls
- [ ] Atomic writes for file creation
- [ ] Path validation for user input
- [ ] No flag shorthand conflicts
- [ ] Progress messages for long operations
- [ ] Table-driven tests for transforms

## Related Documentation

- [Command Structure Refactoring](../architecture/command-structure-refactoring.md) - PersistentPreRunE patterns
- [Security Hardening](../security-issues/cli-security-hardening-input-validation.md) - Path validation patterns
- [CLI Output Format Flag](cli-output-format-flag.md) - Global --output flag handling
- [Auth Middleware Mismatch](../integration-issues/cli-api-auth-middleware-mismatch.md) - v1/v2 API patterns

## Origin

- **Plan:** [docs/plans/2026-03-16-feat-course-module-import-export-plan.md](../../plans/2026-03-16-feat-course-module-import-export-plan.md)
- **Brainstorm:** [docs/brainstorms/2026-03-16-course-module-import-export-brainstorm.md](../../brainstorms/2026-03-16-course-module-import-export-brainstorm.md)
