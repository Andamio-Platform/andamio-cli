# Plan: Content Sync for Andamio CLI

## Problem

Course content exists in two places:
1. **Andamio API** - Edited via Studio UI, served to learners
2. **Local files** - Edited in IDE, tracked with git

We need bidirectional sync with conflict detection.

## Current State

The CLI can already:
- `course list` / `course get` / `course modules` - Read course metadata
- `course lesson <id> <module> <slt>` - Read lesson content
- `course intro` / `course assignment` - Read module parts

Content format is TipTap JSON (rich text editor format).

## Goals

1. **Pull** - Download course content to local directory
2. **Push** - Upload local changes to API
3. **Status** - Show what's changed locally vs remotely
4. **Guard against conflicts** - Don't overwrite unseen remote changes

## Design

### Sync Manifest (`.andamio-sync.json`)

Each synced directory gets a manifest tracking sync state:

```json
{
  "course_id": "6348bba0...",
  "last_sync": "2024-03-13T15:30:00Z",
  "modules": {
    "101": {
      "slt_hash": "b60b006844...",
      "intro_hash": "a1b2c3d4...",
      "assignment_hash": "e5f6g7h8...",
      "lessons": {
        "1": { "content_hash": "abc123...", "local_modified": false },
        "2": { "content_hash": "def456...", "local_modified": false },
        "3": { "content_hash": "ghi789...", "local_modified": false }
      }
    }
  }
}
```

- `slt_hash` - From API, identifies on-chain SLT version
- `content_hash` - blake2b of lesson content_json, computed locally
- `local_modified` - Set true when local file changes (via file watcher or pre-push check)

### Commands

#### `andamio sync pull <course-id> [--dir <path>]`

1. Fetch course metadata + all modules
2. For each module: fetch intro, assignment, lessons
3. Write to `<dir>/<module>/` as JSON files
4. Create/update `.andamio-sync.json` with current hashes
5. If manifest exists and remote hash differs from stored → warn "Remote changed since last sync"

Options:
- `--force` - Overwrite local changes
- `--dry-run` - Show what would change

#### `andamio sync push [--dir <path>]`

1. Read `.andamio-sync.json` to get course_id and known hashes
2. For each locally modified file:
   a. Fetch current remote hash
   b. If remote hash ≠ stored hash → **conflict**, abort
   c. If remote hash = stored hash → safe to push
3. Upload changed content via API
4. Update manifest with new hashes

Options:
- `--force` - Push even if remote changed (overwrite)
- `--dry-run` - Show what would be pushed

#### `andamio sync status [--dir <path>]`

Compare local files to manifest and remote:

```
Module 101 - Your First API Calls
  intro.json      ✓ synced
  lesson-1.json   ↑ local changes (push to sync)
  lesson-2.json   ↓ remote changes (pull to sync)
  lesson-3.json   ⚠ conflict (both changed)
  assignment.json ✓ synced
```

#### `andamio sync init <course-id> [--dir <path>]`

Initialize a new sync directory without pulling content. Creates manifest only.
Useful when starting fresh content locally.

### File Format Options

The current download saves raw API JSON. Consider:

**Option A: Raw JSON (current)**
- Pro: Exact API format, no conversion
- Con: Hard to edit, verbose

**Option B: Simplified JSON**
- Extract just `content_json` from response
- Easier to diff/edit

**Option C: Markdown with frontmatter**
- Convert TipTap JSON ↔ Markdown
- Pro: Git-friendly, human-editable
- Con: Lossy conversion risk (tables, custom blocks)

**Recommendation:** Start with Option B (simplified JSON). Add markdown conversion later as a separate tool if needed.

### API Requirements

Need these endpoints (check if they exist):
- `PUT /course/{id}/module/{code}/lesson/{slt}` - Update lesson
- `PUT /course/{id}/module/{code}/intro` - Update intro
- `PUT /course/{id}/module/{code}/assignment` - Update assignment

If write endpoints don't exist, this becomes read-only sync (pull only).

## Implementation Order

1. **`sync pull`** - Basic download with manifest creation
2. **`sync status`** - Compare local/remote hashes
3. **`sync push`** - Upload with conflict detection (requires write API)
4. **Markdown conversion** - Optional, separate command

## Open Questions

1. Should we sync at course level or allow module-level granularity?
2. How to handle new modules created locally vs remotely?
3. Do we need file watching for automatic `local_modified` tracking, or check on push?
4. Authentication for write operations - same API key or different permissions?

## Related

- Proof of concept: `lesson-coach-v2/downloaded/andamio-for-developers/`
- CLI repo: `andamio-cli/`
