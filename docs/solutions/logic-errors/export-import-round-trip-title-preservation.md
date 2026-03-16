---
title: "Fix export/import round-trip: missing titles, variable shadowing, SLT matching, and empty lessons"
date: 2026-03-16
category: logic-errors
tags:
  - export
  - import
  - round-trip
  - tiptap
  - markdown
  - slt
  - image-upload
  - title-extraction
  - variable-shadowing
  - api-contract
  - format-guide
components:
  - cmd/andamio/course_export.go
  - cmd/andamio/course_import.go
  - cmd/andamio/course_export_test.go
  - cmd/andamio/course_import_test.go
symptoms:
  - "Exported lesson/intro/assignment markdown files had no H1 heading, losing the title on round-trip"
  - "Import JSON output always showed imagesUploaded as 0 due to variable shadowing with := inside inner scope"
  - "SLT heading match was case-sensitive (required exact '## SLT') instead of case-insensitive per format guide"
  - "SLT heading prefix match was too permissive -- '## SLT' prefix matched unrelated headings like '## Slithering'"
  - "Import required lesson count to exactly match SLT count, failing when lessons were intentionally omitted"
  - "Sending an empty lessons array to the API deleted all existing lessons instead of preserving them"
  - "API-sourced titles containing newlines could break markdown structure when embedded as H1 headings"
root_cause: "Export did not write titles as H1 headings; import had variable shadowing, overly strict validation, case-sensitive matching, and unsafe empty-array API semantics"
severity: high
time_to_resolve: "3 hours"
---

# Fix Export/Import Round-Trip: Missing Titles, Variable Shadowing, SLT Matching, and Empty Lessons

## Problem

The andamio-cli export/import round-trip was broken in multiple subtle ways. Running `andamio course export` then `andamio course import` on the same module lost lesson titles, misreported image upload counts, and could silently delete existing lesson content. Additionally, the import command didn't comply with the published Import Format Guide on two points: SLT heading case sensitivity and lesson file optionality.

## Root Causes

| What | Why |
|------|-----|
| Exported markdown files had no H1 title | `convertLessonToMarkdown` and `convertContentToMarkdown` only converted `content_json`, ignoring the `title` field from the API |
| `ImportResult.ImagesUploaded` always 0 | `imagesUploaded, failed := uploadNewImages(...)` used `:=` inside an `if` block, creating a new scoped variable that shadowed the outer one |
| Import rejected modules with fewer lessons than SLTs | Strict equality check treated optional lessons as an error |
| SLT section not found with varied casing | `strings.HasPrefix(trimmed, "## SLT")` was case-sensitive |
| SLT heading matched unrelated headings | Prefix match on `"## SLT"` also matched `"## SLT-Based Learning"`, `"## Slithering"` |
| Importing with no lesson files deleted all existing lessons | Empty `lessons: []` array always sent in payload; API interprets array presence as "replace all" |
| Titles with newlines broke markdown structure | No sanitization before embedding in `# Title\n\n` |
| `TestGenerateOutline` failing | Expected unquoted YAML but code uses `%q` formatting |
| `TestMarkdownToTiptapImage` failing | Expected `image` inside `paragraph` but solo images produce `imageBlock` |

## Solution

### Fix 1: Export titles as H1 headings

Extract `title` from API alongside `content_json` in `fetchModuleData`. Prepend as sanitized H1 in markdown:

```go
// In convertLessonToMarkdown:
md, urls := tiptapToMarkdown(contentJSON)
if title, ok := lesson["title"].(string); ok && title != "" {
    md = "# " + sanitizeTitle(title) + "\n\n" + md
}
return md, urls

// sanitizeTitle strips newlines to prevent markdown structure breakout
func sanitizeTitle(s string) string {
    s = strings.ReplaceAll(s, "\n", " ")
    s = strings.ReplaceAll(s, "\r", " ")
    return strings.TrimSpace(s)
}
```

Same pattern applied in `convertContentToMarkdown` for intro/assignment files.

### Fix 2: Fix variable shadowing on image upload count

```go
// Before (bug -- := creates new scoped variable):
imagesUploaded, failed := uploadNewImages(cfg, assetsDir, ...)

// After (fix -- = assigns to outer variable):
var failed []string
imagesUploaded, failed = uploadNewImages(cfg, assetsDir, ...)
```

### Fix 3: Allow fewer lessons than SLTs

Only error when MORE lessons than SLTs (invalid). Fewer is fine per format guide:

```go
if len(data.Lessons) > len(data.SLTs) {
    return nil, fmt.Errorf("found %d lesson files but outline only lists %d SLTs", ...)
}
if len(data.Lessons) < len(data.SLTs) && len(data.Lessons) > 0 {
    fmt.Printf("Note: %d lesson files for %d SLTs (lessons are optional)\n", ...)
}
```

### Fix 4: Case-insensitive exact SLT heading match

```go
// Before (case-sensitive prefix -- too permissive):
if strings.HasPrefix(trimmed, "## SLT") {

// After (case-insensitive exact match):
lower := strings.ToLower(trimmed)
if lower == "## slts" || lower == "## slt" {
```

### Fix 5: Omit lessons from payload when none provided

Per API contract: "Omitted top-level fields = unchanged" vs "array items replace the full entity."

```go
payload := map[string]interface{}{
    "course_id":          courseID,
    "course_module_code": data.ModuleCode,
    "title":              data.Title,
}
// Only include lessons if files were provided (omitting = preserve existing)
if len(lessons) > 0 {
    payload["lessons"] = lessons
}
```

### Test fixes

- `TestGenerateOutline`: Updated to expect `%q`-quoted YAML values
- `TestMarkdownToTiptapImage` and `TestMarkdownToTiptapImageWithManifest`: Updated to expect `imageBlock` instead of `image` inside `paragraph`

## Key Insight

The API contract has two distinct semantics: **omit a field** means "leave unchanged", while **send an empty array** means "replace with nothing" (i.e., delete all). When making array fields optional in the CLI, you must use omission (don't include the key) rather than sending empty arrays, or you'll silently destroy data.

## Prevention Strategies

### 1. Data field omission from API responses
When consuming an API response, explicitly extract every field. Add round-trip tests that assert all expected fields survive export-then-import. Start from the API schema and check off each field.

### 2. Go variable shadowing with `:=`
Run `go vet` with the shadow analyzer. As a code review habit: any `:=` inside an `if`/`for`/`switch` block warrants checking whether the variable exists in an outer scope.

### 3. Hard-coded validation vs. spec
Before adding validation, find the authoritative spec and quote it in a comment. When the spec says "optional", the validation must be permissive.

### 4. Case-sensitive matching when spec says otherwise
Use `strings.EqualFold` or `strings.ToLower` for spec-defined values. In review, flag any `==` on user-supplied strings.

### 5. Empty array vs. omit semantics
Document the API's null/empty/omit contract. In code, only include array fields in the payload when you intend to replace them. Use a pattern like `if len(items) > 0 { payload["key"] = items }`.

### 6. Unsanitized strings in structured output
Any string from an API response embedded in a structured format (Markdown, YAML) must pass through format-appropriate sanitization. For Markdown H1: strip newlines. Write the sanitizer once, test with adversarial inputs.

## Test Cases to Add

- `TestExtractH1Title` -- edge cases: blank lines before H1, no H1, H2 at top, empty input
- `TestParseSLTsFromOutline` -- case variants: `## slts`, `## Slt`, `## SLTS`, and negative: `## SLT Outcomes`
- `TestImportModuleWithFewerLessons` -- 5 SLTs, 3 lessons: must succeed
- `TestImportModuleWithZeroLessons` -- SLTs but no lesson files: must succeed, payload must omit `lessons` key
- `TestOmitLessonsWhenNoneProvided` -- verify API payload has no `lessons` key when 0 files
- `TestSanitizeTitleNewlines` -- title with `\n`, `\r\n`: must produce single-line H1
- `TestExportImportRoundTrip` -- export a module, re-import, verify titles and content match

## Related Documentation

- [CLI Course Import: App Parity and Payload Alignment](../integration-issues/cli-course-import-app-parity-and-payload-alignment.md) -- H1 title extraction, metadata preservation, imageBlock node type, SLT locking
- [Image Manifest Preserves CDN URLs During Round-Trip](../feature-implementations/cli-course-import-image-manifest-roundtrip.md) -- manifest mechanism, pre-processing approach, load ordering
- [CLI Course Import: Markdown-to-Tiptap Conversion](../feature-implementations/cli-course-import-markdown-to-tiptap-conversion.md) -- goldmark AST walker, node mapping, inline marks
- [CLI Course Export: Tiptap-to-Markdown Conversion](../feature-implementations/cli-course-export-tiptap-conversion.md) -- reverse conversion, image download, atomic writes
- [Course Export Empty Files: API Response Structure Mismatch](../integration-issues/cli-export-api-response-structure-mismatch.md) -- nested response structures, silent data extraction failures
- [Goldmark AST Walker Prevention Strategies](../architecture/goldmark-ast-walker-prevention-strategies.md) -- block vs inline patterns, testing matrix
