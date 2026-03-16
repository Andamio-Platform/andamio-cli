---
title: "feat: Course Module Import/Export"
type: feat
status: active
date: 2026-03-16
deepened: 2026-03-16
origin: docs/brainstorms/2026-03-16-course-module-import-export-brainstorm.md
---

# Course Module Import/Export

## Enhancement Summary

**Deepened on:** 2026-03-16
**Research agents used:** security-sentinel, performance-oracle, architecture-strategist, code-simplicity-reviewer, pattern-recognition-specialist, markdown-tiptap-research, file-operations-research, learnings-synthesis

### Key Improvements
1. **Simplified architecture** - Reduced from 8 new files to 2 (per simplicity review)
2. **Security hardening** - Path traversal prevention, symlink safety, image validation
3. **Performance optimization** - Parallel API calls, concurrent image downloads
4. **Concrete code patterns** - Ready-to-use Go implementations for all operations

### Critical Insights Discovered
- Use `map[string]interface{}` for Tiptap JSON (no custom types needed)
- Atomic file writes via temp-then-rename pattern
- Security: validate image magic bytes, not just extensions
- YAML parsing must use concrete struct types (prevent injection)

---

## Overview

Add `course export` and `course import` commands to the CLI that use the exact same directory structure as the lesson-coach `/compile` skill. This enables Pioneers to:

1. Export existing modules for local editing
2. Import compiled modules from lesson-coach directly
3. Round-trip edit: export → modify in editor → re-import

## Problem Statement / Motivation

Andamio Pioneers use lesson-coach to create course content, which compiles to a `compiled/` directory structure. Currently, there's no way to:

- Get course content from the platform in an editable format
- Upload compiled content without using the web UI
- Make bulk edits locally and sync back

This creates friction for Pioneers who want to work in their preferred editors and version control their course content.

## Proposed Solution

Two new commands under `andamio course`:

```bash
# Export a module to compiled/ format
andamio course export <course-id> <module-code> [--output-dir <path>]

# Import a compiled module directory
andamio course import <path-to-module-dir> --course-id <course-id>
```

Both commands require user JWT authentication (via `andamio user login`).

## Technical Approach

### Architecture (Simplified)

Per simplicity review, consolidate to 2 new files instead of 8:

```
cmd/andamio/
├── course.go              # Existing - add export/import subcommands
├── course_export.go       # NEW: ~200 LOC - export command + helpers
└── course_import.go       # NEW: ~200 LOC - import command + helpers
```

**Rationale:** The original plan proposed 3 new packages with 8 files for a ~400 LOC feature. This is over-engineering for a CLI that's currently 1,700 LOC total. Keep helper functions in the command files; extract to packages only if reuse emerges.

### Research Insights

#### Goldmark for Markdown Parsing
**Why goldmark over blackfriday:**
- Full CommonMark compliance
- Clean AST using interfaces (essential for custom renderers)
- Active maintenance, used by Hugo
- Built-in GFM extensions (tables, strikethrough, task lists)

```go
// Custom renderer pattern for Tiptap output
type TiptapRenderer struct{}

func (r *TiptapRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
    reg.Register(ast.KindParagraph, r.renderParagraph)
    reg.Register(ast.KindHeading, r.renderHeading)
    // ... etc
}
```

#### Tiptap JSON Structure
```json
{
  "type": "doc",
  "content": [
    {
      "type": "paragraph",
      "content": [
        {"type": "text", "text": "Hello", "marks": [{"type": "bold"}]}
      ]
    }
  ]
}
```

**Use `map[string]interface{}`** - No need for custom Go types. The polymorphic node structure works better with dynamic JSON handling.

### Implementation Phases (Consolidated)

#### Phase 1: Export Command

**Tasks:**
- [x] Add `courseExportCmd` to `cmd/andamio/course_export.go`
- [x] Implement `tiptapToMarkdown()` recursive converter
- [x] Implement `writeCompiledModule()` with atomic writes
- [x] Implement parallel image downloads with progress
- [x] Add security: path validation, symlink rejection

**Security Requirements (from security review):**
```go
// Path traversal prevention - REQUIRED
func safePath(baseDir, userPath string) (string, error) {
    cleaned := filepath.Clean(userPath)
    if filepath.IsAbs(cleaned) || strings.HasPrefix(cleaned, "..") {
        return "", fmt.Errorf("invalid path: %s", userPath)
    }
    result := filepath.Join(baseDir, cleaned)
    if !strings.HasPrefix(result, filepath.Clean(baseDir)+string(os.PathSeparator)) {
        return "", fmt.Errorf("path escape detected")
    }
    return result, nil
}
```

**Performance Requirements (from performance review):**
```go
// Parallel lesson fetching - fetch all lessons concurrently
func fetchLessonsParallel(client *Client, courseID string, slts []SLT) ([]Lesson, error) {
    sem := make(chan struct{}, 5) // 5 concurrent requests
    var wg sync.WaitGroup
    // ... see performance review for full implementation
}
```

**Atomic File Writes:**
```go
// Write to temp, rename on success
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
    dir := filepath.Dir(path)
    tmp, err := os.CreateTemp(dir, ".tmp-*")
    if err != nil {
        return err
    }
    defer func() {
        if tmp != nil {
            tmp.Close()
            os.Remove(tmp.Name())
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
    tmp = nil // Prevent cleanup
    return os.Rename(tmp.Name(), path)
}
```

**Success criteria:** `andamio course export <course-id> <module-code>` produces valid compiled/ directory

#### Phase 2: Import Command

**Tasks:**
- [ ] Add `courseImportCmd` to `cmd/andamio/course_import.go`
- [ ] Implement `readCompiledModule()` with YAML frontmatter parsing
- [ ] Implement `markdownToTiptap()` using goldmark custom renderer
- [ ] Implement image upload with magic byte validation
- [ ] Add security: input validation, size limits

**YAML Frontmatter Parsing (secure):**
```go
// Use concrete struct - NEVER map[string]interface{} for YAML
type ModuleOutline struct {
    Title string `yaml:"title"`
    Code  string `yaml:"code"`
}

func parseOutline(data []byte) (*ModuleOutline, string, error) {
    var outline ModuleOutline
    content, err := frontmatter.Parse(bytes.NewReader(data), &outline)
    if err != nil {
        return nil, "", fmt.Errorf("parsing frontmatter: %w", err)
    }
    return &outline, string(content), nil
}
```

**Image Validation (security requirement):**
```go
// Validate by magic bytes, NOT extension
func isValidImage(data []byte) bool {
    if len(data) < 8 {
        return false
    }
    // PNG: 89 50 4E 47 0D 0A 1A 0A
    png := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
    // JPEG: FF D8 FF
    jpeg := []byte{0xFF, 0xD8, 0xFF}

    return bytes.HasPrefix(data, png) || bytes.HasPrefix(data, jpeg)
}

const maxImageSize = 10 * 1024 * 1024 // 10MB per image
```

**Success criteria:** Can import a coach-compiled module to update an existing course module

#### Phase 3: Testing

**Tasks:**
- [x] Table-driven unit tests for conversion functions
- [ ] Golden file tests (sample markdown ↔ JSON pairs)
- [ ] Roundtrip test: export → import produces equivalent content
- [ ] Fuzz tests for malformed input handling
- [ ] Manual test with real preprod course

**Testing Patterns:**
```go
func TestMarkdownToTiptap(t *testing.T) {
    tests := []struct {
        name     string
        markdown string
        want     string
    }{
        {"simple paragraph", "Hello world", `{"type":"doc",...}`},
        {"bold text", "**bold**", `...`},
        {"heading", "## Heading", `...`},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := markdownToTiptap(tt.markdown)
            if !jsonEqual(got, tt.want) {
                t.Errorf("got %v, want %v", got, tt.want)
            }
        })
    }
}
```

## System-Wide Impact

### Interaction Graph

```
course export                    course import
     |                                |
     v                                v
[Config.Load()]              [Config.Load()]
     |                                |
     v                                v
[Client.Get() x N]           [readCompiledModule()]
  (parallel)                         |
     |                               v
     v                       [markdownToTiptap()]
[tiptapToMarkdown()]                 |
     |                               v
     v                       [uploadImages()] (parallel)
[writeCompiledModule()]              |
  (atomic writes)                    v
                             [Client.Post(update)]
```

### Error Propagation

| Error Source | Handling | User Message |
|--------------|----------|--------------|
| JWT missing/expired | Fail fast at command start | "Not authenticated. Run 'andamio user login' first" |
| API 404 (module not found) | Fail with clear error | "Module '101' not found in course 'abc123'" |
| API 403 (not teacher) | Fail with clear error | "You don't have teacher access to this course" |
| Network timeout | Retry once, then fail | "Connection timeout. Please try again." |
| Image download failure | Skip image, warn user | "Warning: Could not download image.png, skipping" |
| Directory exists | Fail unless --force | "Output directory exists. Use --force to overwrite" |
| Invalid YAML frontmatter | Fail with line number | "Invalid outline.md: missing 'code' field at line 3" |
| SLT count mismatch | Fail with details | "Found 5 lesson files but outline.md lists 3 SLTs" |
| Image upload failure | Fail with filename | "Failed to upload assets/diagram.png: 413 entity too large" |
| Path traversal attempt | Reject immediately | "Invalid path: contains directory traversal" |
| Symlink detected | Reject | "Refusing to follow symlink at: path" |
| Invalid image type | Reject | "Invalid image format: expected PNG/JPEG" |

### State Lifecycle Risks

- **Partial export:** Mitigated by atomic writes (write to temp dir, rename on success)
- **Partial import:** If API update fails after image uploads, images orphaned on CDN (acceptable loss)
- **Concurrent edits:** Last write wins; document this behavior

## Security Checklist

From security review - all items REQUIRED before merge:

- [ ] All file paths validated using `safePath()` before read/write
- [ ] Symlinks rejected during import directory traversal (use `os.Lstat()`)
- [ ] Image files validated by magic bytes, not extension
- [ ] Maximum file size limits enforced (10MB/image, 100MB total)
- [ ] Image download URLs validated (HTTPS only, trusted domains)
- [ ] YAML parsing uses concrete struct types
- [ ] Course IDs escaped with `url.PathEscape()` before API calls
- [ ] Temporary files created with 0600 permissions
- [ ] No credentials in error messages
- [ ] `--force` flag warns user before overwriting

## Performance Targets

From performance review:

| Operation | Target | Approach |
|-----------|--------|----------|
| Export (5 lessons, 10 images) | <30s | Parallel lesson fetch (5 concurrent) |
| Import (10 images) | <60s | Parallel image upload (3-5 concurrent) |
| Single API call | <500ms avg | Existing client timeout (30s) is sufficient |

**HTTP Client Enhancement (recommended):**
```go
transport := &http.Transport{
    MaxIdleConnsPerHost: 10,
    MaxConnsPerHost:     10,
    IdleConnTimeout:     90 * time.Second,
}
```

## Acceptance Criteria

### Functional Requirements

- [ ] `andamio course export <course-id> <module-code>` exports module to `compiled/` format
- [ ] `andamio course import <path> --course-id <id>` imports module from `compiled/` format
- [ ] Export output matches coach `/compile` format exactly
- [ ] Import accepts coach-compiled modules without modification
- [ ] Images are downloaded during export and uploaded during import
- [ ] Both commands require user JWT authentication

### Non-Functional Requirements

- [ ] Export completes in <30 seconds for typical module
- [ ] Import completes in <60 seconds including image uploads
- [ ] No data loss for standard Markdown/Tiptap constructs
- [ ] Clear error messages for all failure modes
- [ ] All security checklist items pass

### Quality Gates

- [ ] Unit tests for conversion functions (>80% coverage)
- [ ] Golden file tests for known markdown/JSON pairs
- [ ] Security test: path traversal attempts rejected
- [ ] Manual test with real preprod course
- [ ] README updated with examples

## Dependencies & Risks

### Dependencies

| Dependency | Status | Notes |
|------------|--------|-------|
| goldmark | Available | `github.com/yuin/goldmark` - CommonMark parser |
| frontmatter | Available | `github.com/adrg/frontmatter` - YAML frontmatter |
| CDN upload endpoint | **Unknown** | Need to investigate web app |
| Teacher API access | Available | Existing endpoints work |
| User JWT auth | Available | Already implemented |

### Risks

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| CDN upload endpoint doesn't exist | Medium | High | Check web app network tab; may need API team support |
| Tiptap custom nodes not convertible | Medium | Medium | Document unsupported types; log warnings |
| Large modules timeout | Low | Medium | Parallel fetching addresses this |

## Out of Scope

Per brainstorm decisions:

- Course-level import (all modules at once) - future enhancement
- Creating new modules (only updating existing)
- Video upload - videos must be pre-hosted
- `--dry-run` flag - defer to Phase 2 if users request
- Progress bars with fancy UI - simple print statements sufficient

## Learnings Applied

From `docs/solutions/`:

1. **Auth headers conflict** - Import POST must check for JWT; API rejects both headers simultaneously
2. **Output format flag** - Use `output.Print()` for confirmation messages to respect `--output` flag
3. **Command structure** - Follow existing `init()` registration, `PersistentPreRunE` for auth
4. **Security hardening** - Use `url.PathEscape()`, 0600 permissions, validate URLs

## Sources & References

### Origin

- **Brainstorm document:** [docs/brainstorms/2026-03-16-course-module-import-export-brainstorm.md](../brainstorms/2026-03-16-course-module-import-export-brainstorm.md)
- Key decisions: Coach format, CLI conversion, CDN upload, module-level granularity

### Internal References

- Command patterns: `cmd/andamio/course.go`
- Auth pattern: `cmd/andamio/teacher.go:10-27`
- File I/O pattern: `cmd/andamio/spec.go:56-65`
- Security patterns: `docs/solutions/security-issues/cli-security-hardening-input-validation.md`

### External References

- goldmark: https://github.com/yuin/goldmark
- goldmark renderer API: https://pkg.go.dev/github.com/yuin/goldmark/renderer
- Tiptap schema: https://tiptap.dev/docs/editor/core-concepts/schema
- Atomic writes in Go: https://pkg.go.dev/github.com/google/renameio
- adrg/frontmatter: https://pkg.go.dev/github.com/adrg/frontmatter

### Related Work

- Coach compile skill: `~/projects/01-projects/andamio-lesson-coach-v2/.claude/skills/compile/SKILL.md`
- API spec: `./openapi.json`

## Open Items

1. **CDN upload endpoint** - Need to investigate how web app uploads images:
   - Check network tab when uploading image in Tiptap editor
   - Look for `/v2/media/upload` or similar
   - May require signed URL workflow

2. **Tiptap node inventory** - Catalog all node types used in Andamio courses:
   - Known: doc, paragraph, heading, bulletList, orderedList, listItem, blockquote, codeBlock, image, hardBreak, horizontalRule
   - Known marks: bold, italic, code, link, strike
   - Unknown: custom callout blocks? embeds?
