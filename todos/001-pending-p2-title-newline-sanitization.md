---
status: complete
priority: p2
issue_id: "001"
tags: [code-review, security, export, sanitization]
dependencies: []
---

# Title Newline Sanitization in Export

## Problem Statement

When exporting, title strings from the API are prepended directly into markdown files as H1 headings:

```go
md = "# " + title + "\n\n" + md
```

If a title contains newline characters, the H1 heading structure breaks apart. A title like `"Legit Title\n\n[evil](http://bad.com)\n\n# "` would produce markdown where injected content escapes the heading context. On re-import, `extractH1Title` would parse only `"Legit Title"` as the title, but the injected content would be parsed by goldmark and included in the Tiptap JSON body.

This was flagged by the security reviewer as medium severity. The attack surface is limited (API is authenticated, teacher endpoint only), but defense-in-depth is warranted.

## Findings

- **Source**: Security Sentinel agent
- **Location**: `cmd/andamio/course_export.go` lines 496-497 and 533-534
- **Evidence**: The `title` string from API is used directly in string concatenation without sanitization
- **Mitigating factors**: API requires teacher auth, goldmark strips raw HTML, Tiptap sanitizes on render
- **Known Pattern**: docs/solutions/ documents previous API structure mismatch issues - this is another case where trusting API data shape causes problems

## Proposed Solutions

### Option A: Strip newlines from title (Recommended)
Add a simple sanitizer before prepending:

```go
func sanitizeTitle(s string) string {
    s = strings.ReplaceAll(s, "\n", " ")
    s = strings.ReplaceAll(s, "\r", " ")
    return strings.TrimSpace(s)
}
```

Apply in both `convertLessonToMarkdown` and `convertContentToMarkdown`.

- **Pros**: Minimal change, prevents the structural breakout, handles the most impactful attack vector
- **Cons**: Doesn't sanitize markdown syntax chars in titles (links, images) -- but these would require a compromised API to exploit
- **Effort**: Small
- **Risk**: Low

### Option B: Full markdown escaping of title
Escape all markdown-significant characters in the title before embedding.

- **Pros**: Comprehensive protection against any markdown injection
- **Cons**: Over-engineered for the threat model; may break legitimate titles with special characters
- **Effort**: Small-Medium
- **Risk**: Low-Medium (could break existing titles with backticks, brackets, etc.)

## Recommended Action

Option A -- strip newlines only. This prevents the H1 breakout which is the actual vulnerability. Further markdown escaping is unnecessary given the authenticated API surface.

## Technical Details

**Affected files:**
- `cmd/andamio/course_export.go` (convertLessonToMarkdown, convertContentToMarkdown)

## Acceptance Criteria

- [ ] Title strings with `\n` or `\r` are sanitized before markdown embedding
- [ ] Existing titles without newlines are unaffected
- [ ] Round-trip test: export with clean title -> import preserves title correctly

## Work Log

| Date | Action | Learnings |
|------|--------|-----------|
| 2026-03-16 | Identified via security review | API-sourced strings need boundary sanitization before embedding in structured formats |

## Resources

- Security Sentinel review of export/import changes
- docs/solutions/integration-issues/cli-export-api-response-structure-mismatch.md
