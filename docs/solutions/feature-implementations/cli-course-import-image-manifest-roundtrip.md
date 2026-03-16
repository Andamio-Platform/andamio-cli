---
title: "Image manifest preserves CDN URLs during export/import round-trip"
category: feature-implementations
tags: [image-manifest, goldmark, export-import-round-trip, tiptap, pre-processing, url-validation]
module: course-import
symptom: "After exporting a course module with images, editing locally, and re-importing, images are replaced with placeholder text '[Image: alt]' instead of preserving original CDN URLs"
root_cause: "Export downloads images to local assets/ and rewrites URLs to relative paths, but import has no way to restore original CDN URLs. Additionally, the initial implementation loaded the manifest AFTER conversion, making it inoperative at runtime."
---

# Image Manifest Preserves CDN URLs During Export/Import Round-Trip

## Overview

The `andamio course export` command downloads images from CDN URLs to local `assets/` and rewrites references to `assets/filename.png` in markdown. When re-importing with `andamio course import`, these local references can't be resolved back to CDN URLs, producing `[Image: alt]` placeholder text. The Andamio API has no image upload endpoint, so the images are lost.

The solution: export writes a `.image-manifest.json` mapping filenames to original CDN URLs. Import reads it and pre-processes markdown to restore URLs before goldmark parsing.

## Solution

### Export: Write Manifest

After downloading images, `writeImageManifest()` creates a JSON file mapping local filenames to their original CDN URLs:

```go
// cmd/andamio/course_export.go
func writeImageManifest(assetsDir string, urls []string) error {
    manifest := make(map[string]string)
    for _, imgURL := range urls {
        filename := filepath.Base(imgURL)
        if idx := strings.Index(filename, "?"); idx != -1 {
            filename = filename[:idx]
        }
        if _, exists := manifest[filename]; !exists {
            manifest[filename] = imgURL
        }
    }
    data, err := json.MarshalIndent(manifest, "", "  ")
    if err != nil {
        return err
    }
    return writeFileAtomic(filepath.Join(assetsDir, ".image-manifest.json"), data)
}
```

Produces:
```json
{
  "diagram.png": "https://cdn.andamio.io/images/abc/diagram.png",
  "screenshot.png": "https://cdn.andamio.io/images/abc/screenshot.png"
}
```

### Import: Pre-Process Markdown

Instead of threading the manifest through 5+ AST converter functions, a single string replacement pass resolves all paths before goldmark parsing:

```go
// cmd/andamio/course_import.go
func markdownToTiptap(md string, manifest map[string]string) (map[string]interface{}, error) {
    md = resolveManifestPaths(md, manifest) // Pre-process ONCE
    // ... goldmark parsing sees only full URLs ...
}

func resolveManifestPaths(md string, manifest map[string]string) string {
    if len(manifest) == 0 {
        return md
    }
    for filename, url := range manifest {
        md = strings.ReplaceAll(md, "assets/"+filename, url)
    }
    return md
}
```

### Import: Load Manifest BEFORE Conversion

Critical ordering — the manifest must be loaded before any `markdownToTiptap` calls:

```go
func readCompiledModule(dir string) (*ImportData, error) {
    data := &ImportData{}
    // ... parse outline.md ...

    // Load image manifest BEFORE converting any content
    assetsDir := filepath.Join(dir, "assets")
    data.ImageManifest = loadImageManifest(assetsDir)

    // NOW convert lessons (manifest is available)
    for _, lessonFile := range lessonFiles {
        tiptap, err := markdownToTiptap(string(content), data.ImageManifest)
        // ...
    }
}
```

### URL Validation on Manifest Values

The manifest is a user-controllable file. Only `http://` and `https://` URLs are accepted:

```go
func loadImageManifest(assetsDir string) map[string]string {
    // ... read and parse JSON ...

    manifest := make(map[string]string, len(raw))
    for filename, url := range raw {
        if strings.HasPrefix(url, "https://") || strings.HasPrefix(url, "http://") {
            manifest[filename] = url
        } else {
            fmt.Printf("Warning: skipping manifest entry with invalid URL scheme: %s\n", filename)
        }
    }
    return manifest
}
```

## The Complete Flow

| Phase | Action | Detail |
|-------|--------|--------|
| Export | Fetch module from API | Tiptap JSON with CDN image URLs |
| Export | Convert Tiptap → Markdown | `![alt](assets/diagram.png)` |
| Export | Download images | Save to `assets/` directory |
| Export | **Write manifest** | `.image-manifest.json` maps `diagram.png` → CDN URL |
| Import | **Load manifest** | Read before any conversions (critical ordering) |
| Import | **Pre-process markdown** | `assets/diagram.png` → `https://cdn.../diagram.png` |
| Import | Parse markdown | Goldmark sees only full URLs |
| Import | Convert AST → Tiptap | Image nodes get proper `src` attributes |
| Import | POST to API | Tiptap JSON with original CDN URLs restored |

## Key Insights

### 1. Pre-Processing Beats Parameter Threading

The initial implementation threaded `manifest` through `convertNode` → `convertSingleNode` → `convertInlineContent` → `convertInlineNode` (5 functions, 8 parameter additions). A simple `strings.ReplaceAll` pass before parsing eliminated all of it with ~40 fewer lines of code.

**When to use pre-processing:** The transformation is string-replaceable and affects the input to multiple downstream functions.

**When to use parameter threading:** The decision depends on internal AST state (e.g., block vs inline context).

### 2. Integration Tests Must Exercise Orchestration

The manifest ordering bug (loaded after conversion) was caught only by review agents, not unit tests. Unit tests called `markdownToTiptap(md, manifest)` directly with a pre-loaded manifest, bypassing `readCompiledModule` where the ordering bug lived.

The fix: `TestReadCompiledModuleWithManifest` creates a real directory with manifest and verifies the full flow produces resolved URLs, not placeholders.

### 3. Distinguish File-Not-Found from Parse Errors

`loadImageManifest` silently returned an empty map for ALL errors. A corrupted manifest would produce no warning — images would silently become placeholders. The fix: warn on parse errors, silence only `os.IsNotExist`.

### 4. Validate User-Controllable File Content

Manifest URLs go into Tiptap JSON sent to the API and rendered to other users. Without validation, a tampered manifest could inject `javascript:` URIs or `data:` URIs. Scheme allowlisting (`http://`/`https://` only) prevents this.

## Prevention Strategies

### Before Writing Context-Dependent Code

- Map dependency order: what must be loaded before what
- If a value is used in 5+ functions, consider pre-processing instead of threading
- Document ordering requirements in comments

### Before Merging

- Add at least one integration test that exercises the full orchestration flow
- Test negative paths: missing file, malformed input, malicious content
- Verify error handling distinguishes expected vs unexpected failures

### Common Mistakes

| Mistake | Symptom | Prevention |
|---------|---------|-----------|
| Manifest loaded after use | Images show as placeholders despite manifest existing | Integration test through `readCompiledModule` |
| Threading manifest through 5+ functions | Over-engineered, hard to modify | Pre-process with `strings.ReplaceAll` at entry point |
| No URL validation on manifest | XSS/injection via manifest | Allowlist `http://`/`https://` schemes |
| Silent error swallowing | Corrupted manifest produces no warning | Distinguish file-not-found from parse errors |
| Unit tests bypass orchestration | Ordering bugs invisible | Black-box integration tests through public functions |

## Testing

Key test cases in `cmd/andamio/course_import_test.go`:

- `TestReadCompiledModuleWithManifest` — integration test: creates full directory with manifest, verifies resolved URLs in output
- `TestResolveManifestPaths` — string replacement: multiple images, unknown assets, external URLs unchanged
- `TestLoadImageManifestURLValidation` — rejects `javascript:`, `data:` schemes; accepts `http://`, `https://`
- `TestMarkdownToTiptapImageWithManifest` — end-to-end: manifest → pre-process → parse → proper image node
- `TestMarkdownToTiptapImageWithoutManifest` — fallback: local image → placeholder text

## Related Documentation

- [CLI Course Export: Tiptap-to-Markdown Conversion](cli-course-export-tiptap-conversion.md) — Export side, image download, atomic writes
- [CLI Course Import: Markdown-to-Tiptap Conversion](cli-course-import-markdown-to-tiptap-conversion.md) — Import side, AST walking, node mapping
- [Goldmark AST Walker Prevention Strategies](../architecture/goldmark-ast-walker-prevention-strategies.md) — Image URL differentiation, block vs inline handling
- [CLI Export API Response Structure Mismatch](../integration-issues/cli-export-api-response-structure-mismatch.md) — API response nesting for lessons/images
- [CLI Security Hardening](../security-issues/cli-security-hardening-input-validation.md) — URL validation, file permission patterns

## Files

- Export manifest: `cmd/andamio/course_export.go` (`writeImageManifest`)
- Import manifest: `cmd/andamio/course_import.go` (`loadImageManifest`, `resolveManifestPaths`)
- Tests: `cmd/andamio/course_import_test.go`
- Plan: `docs/plans/2026-03-16-feat-course-import-image-upload-plan.md`
