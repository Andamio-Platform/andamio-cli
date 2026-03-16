# Brainstorm: Course Module Import/Export

**Date:** 2026-03-16
**Status:** Ready for planning

## What We're Building

CLI commands to export and import course modules using the same directory structure that `andamio-lesson-coach` produces with its `/compile` skill. This enables a round-trip workflow: export → edit locally → re-import.

### New Commands

```bash
# Export a module to compiled/ format
andamio course export <course-id> <module-code> [--output-dir <path>]

# Import a compiled module directory
andamio course import <path-to-module-dir> --course-id <course-id>
```

### The Compiled Module Format

Matches coach's `/compile` output exactly:

```
compiled/[course-slug]/[module-code]/
├── outline.md          # YAML frontmatter (title, code) + ## SLTs list
├── introduction.md     # Optional module intro
├── lesson-1.md         # Lesson for SLT 1
├── lesson-2.md         # Lesson for SLT 2
├── lesson-N.md         # Lesson for SLT N
├── assignment.md       # Optional module assignment
└── assets/             # Images referenced in lessons
    └── *.png
```

### outline.md Format

```markdown
---
title: Module Title
code: 101
---

# Module Title

## SLTs

1. First learning outcome text
2. Second learning outcome text
3. Third learning outcome text
```

## Why This Approach

1. **Compatibility with coach** - Pioneers already have compiled modules from lesson-coach; they can import directly without format conversion
2. **Human-readable** - Markdown files are easy to edit in any editor, diff in git
3. **Round-trip editing** - Export existing courses, modify locally, re-import changes
4. **Single source of truth** - One format for both coach and CLI

## Key Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Format | Coach's `compiled/` directory structure | Direct compatibility, no translation layer |
| Conversion location | CLI (Go) | API stays focused on Tiptap JSON; conversion logic in client |
| Image handling | Upload to Andamio CDN via API | Full self-contained workflow |
| Granularity | Module-level only | Matches coach output; simpler implementation |
| Primary use case | Round-trip editing | Format fidelity is critical for export→edit→import |

## Technical Requirements

### Export Command

1. Fetch module data via `GET /api/v2/course/teacher/...` endpoints
2. Convert Tiptap JSON → Markdown
3. Write outline.md with YAML frontmatter
4. Write lesson-N.md files (one per SLT)
5. Write introduction.md and assignment.md if present
6. Download images from URLs, save to assets/

### Import Command

1. Parse outline.md for module code, title, SLTs
2. Read lesson-N.md files
3. Convert Markdown → Tiptap JSON
4. Upload images from assets/ to CDN, get hosted URLs
5. Rewrite image references in content
6. Call `POST /v2/course/teacher/course-module/update`

### Markdown ↔ Tiptap Conversion

Need to implement bidirectional conversion:
- **Export (Tiptap → Markdown):** Extract text content, preserve headings/lists/links
- **Import (Markdown → Tiptap):** Parse Markdown AST, build Tiptap JSON nodes

Options:
- Use a Go Markdown parser (goldmark, blackfriday) and build Tiptap JSON manually
- Shell out to a Node.js script if conversion is complex
- Find existing Go library for Tiptap/ProseMirror

## Open Questions

None - all key decisions resolved.

## Out of Scope

- Course-level import (all modules at once) - future enhancement
- Creating new modules (only updating existing)
- Video upload - videos must be pre-hosted

## Related

- **Coach compile skill:** `~/projects/01-projects/andamio-lesson-coach-v2/.claude/skills/compile/SKILL.md`
- **API endpoint:** `POST /v2/course/teacher/course-module/update`
- **Tiptap format:** ProseMirror-based JSON schema
