---
title: "CLI Course Import: App Parity and Payload Alignment"
date: 2026-03-16
category: integration-issues
tags:
  - course-import
  - course-export
  - tiptap-json
  - goldmark
  - api-payload
  - teacher-endpoints
  - image-manifest
  - metadata-preservation
components:
  - cmd/andamio/course_export.go
  - cmd/andamio/course_import.go
  - cmd/andamio/main.go
symptoms:
  - "Export fails with 404 for draft/unpublished modules"
  - "Import wipes lesson content, titles, and metadata"
  - "Bullet point text silently disappears after import"
  - "Images render incorrectly or are ignored after import"
  - "Existing metadata (description, image_url, video_url) lost on update"
root_cause: "Multiple integration gaps between CLI export/import and the Andamio web app's module-upload feature"
severity: high
related_issues:
  - https://github.com/Andamio-Platform/andamio-cli/issues/7
related_docs:
  - docs/solutions/feature-implementations/cli-course-export-tiptap-conversion.md
  - docs/solutions/feature-implementations/cli-course-import-markdown-to-tiptap-conversion.md
  - docs/solutions/feature-implementations/cli-course-import-image-manifest-roundtrip.md
  - docs/solutions/integration-issues/cli-export-api-response-structure-mismatch.md
  - docs/solutions/architecture/goldmark-ast-walker-prevention-strategies.md
---

# CLI Course Import: App Parity and Payload Alignment

## Problem Summary

After building the initial course export/import commands, testing revealed five distinct integration failures that caused data loss, content corruption, or feature gaps when compared to the web app's "Upload Module Folder" feature. Each stemmed from a mismatch between CLI assumptions and the actual API contracts or Tiptap schema the platform uses.

## Bug 1: Export 404 for Draft Modules

### Symptom

```
$ andamio course export da98d768... 101
Error: failed to fetch SLTs: API error 404: {"error":{"code":"NOT_FOUND","message":"SLTs not found (module not on-chain or does not exist)"}}
```

### Root Cause

The export used user-facing GET endpoints (`/api/v2/course/user/slts/`) that only return data for modules published on-chain. Draft modules (status: DRAFT, APPROVED, PENDING_TX) only exist in the database and are invisible to these endpoints.

### Fix

Replaced all user GET endpoints with a single teacher POST endpoint:

```go
// BEFORE: 4-5 separate GET calls (fail for drafts)
c.Get("/api/v2/course/user/slts/" + courseID + "/" + moduleCode, &sltsResp)
c.Get("/api/v2/course/user/introduction/" + courseID + "/" + moduleCode, &introResp)
c.Get("/api/v2/course/user/assignment/" + courseID + "/" + moduleCode, &assignResp)

// AFTER: 1 POST call (works for all statuses)
c.Post("/api/v2/course/teacher/course-modules/list", map[string]string{"course_id": courseID}, &resp)
// Response includes SLTs, lessons, introduction, assignment — all inline
```

The teacher endpoint returns a union of on-chain and DB modules, including drafts in any status. This also reduced API calls from 4-5 to 2 (one for course slug, one for module data).

---

## Bug 2: Import Wipes Content

### Symptom

After running `course import`, all lesson content, titles, introduction, and assignment were empty in the web app.

### Root Cause (three issues)

**2a. Missing `content_json` wrapper.** The API expects `AggregateIntroductionInput` and `AggregateAssignmentInput` with content nested in a `content_json` field. The CLI sent raw Tiptap JSON directly:

```go
// WRONG: API doesn't recognize these fields, sets content_json to null
payload["introduction"] = tiptapJSON  // {"type": "doc", "content": [...]}

// CORRECT: Wrap in the expected schema
payload["introduction"] = map[string]interface{}{
    "content_json": tiptapJSON,
}
```

**2b. No H1 title extraction.** The app strips the first `# Heading` from each file and uses it as the `title` field. The CLI included the H1 in the body and sent no title, so lessons were created titleless.

```go
// App behavior: H1 → title field, rest → content_json
func extractH1Title(md string) (title string, body string) {
    lines := strings.Split(md, "\n")
    for i, line := range lines {
        trimmed := strings.TrimSpace(line)
        if strings.HasPrefix(trimmed, "# ") && !strings.HasPrefix(trimmed, "## ") {
            title = strings.TrimPrefix(trimmed, "# ")
            body = strings.TrimSpace(strings.Join(lines[i+1:], "\n"))
            return title, body
        }
        if trimmed != "" { break }
    }
    return "", strings.TrimSpace(md)
}
```

**2c. No metadata preservation.** The API uses full-replacement semantics. Sending a lesson with only `slt_index` and `content_json` clears `title`, `description`, `image_url`, and `video_url`. The fix: fetch existing module state before updating and merge metadata fields the CLI doesn't modify.

```go
// Fetch existing state, merge metadata into payload
existing, _ := fetchExistingModule(c, courseID, moduleCode)

for i, lesson := range data.Lessons {
    l := map[string]interface{}{
        "slt_index":    lesson.Index,
        "content_json": lesson.TiptapJSON,
    }
    if lesson.Title != "" {
        l["title"] = lesson.Title
    } else if existing, ok := existing.Lessons[lesson.Index]; ok {
        if v, ok := existing["title"].(string); ok && v != "" {
            l["title"] = v
        }
    }
    // Preserve description, image_url, video_url from existing
    if existing, ok := existing.Lessons[lesson.Index]; ok {
        for _, field := range []string{"description", "image_url", "video_url"} {
            if v, ok := existing[field]; ok && v != nil && v != "" {
                l[field] = v
            }
        }
    }
}
```

---

## Bug 3: Bullet Points Silently Dropped

### Symptom

After import, all bullet list text disappeared. No error or warning. The list markers rendered but with empty content.

### Root Cause

Goldmark uses `ast.TextBlock` (not `ast.Paragraph`) for content inside tight lists (lists with no blank lines between items). The converter only handled `ast.Paragraph`, so tight list content was silently skipped.

### Fix

```go
case *ast.TextBlock:
    // Tight list items use TextBlock, not Paragraph.
    content := convertInlineContent(node, source)
    if len(content) == 0 {
        return nil
    }
    return map[string]interface{}{
        "type":    "paragraph",
        "content": content,
    }
```

---

## Bug 4: Images Render Incorrectly

### Symptom

Images imported via CLI appeared broken or unstyled compared to images uploaded through the web app.

### Root Cause

The CLI generated `{"type": "image"}` nodes. The app uses `{"type": "imageBlock"}` with additional layout attributes (`width`, `align`). Mismatched node types caused rendering issues.

### Fix

```go
func imageBlockNode(src, alt string) map[string]interface{} {
    return map[string]interface{}{
        "type": "imageBlock",
        "attrs": map[string]interface{}{
            "src": src, "alt": alt,
            "width": "600", "align": "center",
        },
    }
}
```

Solo-image paragraphs are detected at the AST level and converted to `imageBlock` nodes, matching the app's `markdown-to-tiptap.ts` behavior.

---

## Bug 5: SLT_LOCKED Error on Approved Modules

### Symptom

```
Error: failed to update module: API error 400: {"code":"SLT_LOCKED","message":"SLTs are locked after approval"}
```

### Root Cause

The import always sent SLTs in the payload. For modules with status APPROVED or ON_CHAIN, SLT definitions are immutable (they're hashed for on-chain identity). Sending them triggers the lock check even if unchanged.

### Fix

Check module status before updating. Skip SLTs when locked:

```go
existing, _ := fetchExistingModule(c, courseID, data.ModuleCode)
sltsLocked := existing.Status != "DRAFT"

if !sltsLocked {
    payload["slts"] = slts
}
// Lessons, introduction, assignment can always be updated
```

---

## Code Review Hardening

After fixing the integration bugs, a multi-agent code review identified additional issues:

| Fix | Description |
|-----|-------------|
| Safe type assertion | `content[0].(map[string]interface{})` changed to two-value form to prevent panics |
| YAML frontmatter quoting | Title and code fields now use `%q` to prevent YAML injection |
| Regex hoisting | 3 regexes moved from inside loops to package-level `var` declarations |
| Manifest warning gating | `loadImageManifest` warnings suppressed in `--output json` mode |
| Introduction metadata | Added `image_url`/`video_url` preservation (was only preserving `description`) |

---

## Prevention Strategies

### 1. Default to teacher endpoints for authoring operations

User endpoints are for consumption. Teacher endpoints return draft + published content. Before implementing any endpoint, run `./andamio spec paths --filter <resource>` and verify the endpoint works for unpublished content.

### 2. Read the OpenAPI schema before writing any POST/PUT handler

The `AggregateUpdateModuleV2Request` schema defines exactly what fields are expected. Define Go structs matching the schema, or at minimum compare your `map[string]interface{}` payload against the spec.

### 3. Implement read-before-write for all update operations

The API uses full-replacement semantics. Always fetch existing state, merge in your changes, and send the complete entity. Never send partial payloads.

### 4. Handle all AST node types explicitly

When writing goldmark AST converters, enumerate all node types the library can produce. The `default` case should log unhandled types, never silently skip. Test with both tight and loose list variants.

### 5. Use the app's actual JSON output as the Tiptap schema reference

Don't assume standard node types. Export content from the web app, inspect the JSON, and match the CLI's output to the app's format exactly (`imageBlock` not `image`, etc.).

### 6. Round-trip testing is the highest-value test pattern

`export -> edit -> import -> export -> diff` catches payload mismatches, dropped content, metadata loss, and node type errors in a single pass.

---

## Testing Checklist

- [ ] Export succeeds for modules in every status (DRAFT, APPROVED, PENDING_TX, ON_CHAIN)
- [ ] Import preserves existing metadata (title, description, image_url, video_url)
- [ ] Bullet points survive round-trip (tight and loose lists)
- [ ] Images render as `imageBlock` with width/align attrs
- [ ] H1 headings extracted as title field, not included in content_json
- [ ] SLTs skipped for non-DRAFT modules (no SLT_LOCKED error)
- [ ] `--output json` produces clean JSON (no interleaved warnings)
- [ ] Image manifest preserves CDN URLs through round-trip
- [ ] New images (not in manifest) produce clear warnings

---

## Key Insight

The CLI's import must produce **exactly the same API payload** as the web app's "Upload Module Folder" feature. The source of truth is `andamio-app-v2/src/lib/markdown-to-tiptap.ts` and `andamio-app-v2/src/hooks/api/course/use-save-module-draft.ts`. Any divergence between CLI and app payload structure will cause content corruption.
