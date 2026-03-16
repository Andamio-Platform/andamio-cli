---
title: "feat: Course Import Image Upload"
type: feat
status: active
date: 2026-03-16
origin: docs/brainstorms/2026-03-16-course-module-import-export-brainstorm.md
---

# Course Import Image Upload

## Overview

The `andamio course import` command currently skips local images with a `[Image: alt]` placeholder, breaking the round-trip workflow (export -> edit -> import). This plan addresses getting images uploaded and properly referenced in imported course content.

## Problem Statement

The round-trip workflow has a gap:

1. **Export** downloads images from CDN URLs to `assets/filename.png` and rewrites Tiptap image `src` to local `assets/` paths in markdown
2. **Edit** — user modifies content locally, may add new images to `assets/`
3. **Import** encounters `![alt](assets/filename.png)` references but **cannot upload them** — replaces with `[Image: alt]` text placeholder

Additionally, during export the original CDN URLs are **lost** — no metadata preserves them, so even unchanged images can't be restored on import.

## Root Cause Analysis

**No image upload endpoint in the Andamio API.** The entire OpenAPI spec has zero multipart/form-data or file upload endpoints. All `image_url` fields across modules, lessons, assignments, and introductions accept only pre-hosted URL strings. The web app likely uses a separate upload service (Tiptap's built-in upload, or a CDN-specific endpoint) not exposed in the public API.

## Proposed Solution: Phased Approach

### Phase 1: Preserve Original URLs During Round-Trip (No Upload Needed)

The simplest fix — ensure unchanged images survive the export/import cycle without needing any upload.

**Changes to export (`course_export.go`):**

Store original image URLs in a manifest file alongside the downloaded images:

```
assets/
├── diagram.png
├── screenshot.png
└── .image-manifest.json    # NEW: maps filenames → original URLs
```

`.image-manifest.json` format:
```json
{
  "diagram.png": "https://cdn.andamio.io/images/abc123/diagram.png",
  "screenshot.png": "https://cdn.andamio.io/images/abc123/screenshot.png"
}
```

**Changes to import (`course_import.go`):**

1. Read `.image-manifest.json` if it exists
2. For each `![alt](assets/filename.png)` in markdown:
   - If filename exists in manifest → use the original CDN URL in the Tiptap `image` node
   - If filename NOT in manifest → this is a new image (user-added), warn as before
3. Emit proper Tiptap image nodes with `src` set to the original URL

This preserves existing images through the round-trip with **zero API changes required**.

### Phase 2: External Image Hosting for New Images

For images added during local editing that don't have original URLs, provide a `--image-host` flag:

```bash
andamio course import ./compiled/my-course/101 --course-id abc123 --image-host s3
```

**Strategy options (user chooses one):**

| Strategy | Flag Value | How It Works |
|----------|-----------|--------------|
| Skip (current) | `--image-host skip` (default) | Warn and use placeholder |
| Pre-hosted URL | `--image-host url` | User puts full URLs in markdown `![alt](https://...)` — already works |
| S3/R2 upload | `--image-host s3` | Upload to user's S3/R2 bucket, rewrite URLs |

The S3/R2 strategy requires additional config:
```json
// ~/.andamio/config.json
{
  "image_host": {
    "type": "s3",
    "bucket": "my-andamio-assets",
    "region": "us-east-1",
    "prefix": "course-images/"
  }
}
```

### Phase 3: Native Andamio CDN Upload (When Available)

If/when the Andamio API exposes an image upload endpoint, add `--image-host andamio` as the default strategy. This would be the ideal end state but is blocked on API support.

## Acceptance Criteria

### Phase 1 (MVP)
- [x] Export writes `.image-manifest.json` to `assets/` with original URLs
- [x] Import reads manifest and restores original URLs for unchanged images
- [x] Images with manifest entries produce proper Tiptap `image` nodes (not placeholders)
- [x] New images (no manifest entry) still warn as before
- [x] Backward compatible — import works fine if no manifest exists

### Phase 2 (Optional)
- [ ] `--image-host url` strategy works (external URLs in markdown pass through)
- [ ] `--image-host s3` uploads to configured bucket and rewrites URLs
- [ ] Config supports `image_host` settings

## Technical Considerations

### Manifest Design
- JSON format for simplicity (no new dependencies)
- Dotfile (`.image-manifest.json`) to avoid cluttering the visible directory
- Export must handle duplicate filenames from different URLs (append hash if collision)

### Image Node in Tiptap JSON
The Tiptap image node format (already partially handled in import):
```json
{
  "type": "image",
  "attrs": {
    "src": "https://cdn.example.com/image.png",
    "alt": "Description"
  }
}
```

### Block-Level vs Inline Images
The current import handles images in two places:
- `convertSingleNode()` (block-level `*ast.Image`) — returns paragraph with placeholder
- `convertInlineNode()` (inline `*ast.Image`) — returns text placeholder for local, image node for URLs

Both need updating to check the manifest.

### Security
- Manifest URLs should only be trusted if they match known CDN domains
- S3 upload (Phase 2) should validate file types via magic bytes, not just extension

## Dependencies & Risks

| Risk | Mitigation |
|------|-----------|
| Manifest file could be manually edited with malicious URLs | Validate URLs match expected CDN patterns |
| Export filename collisions (two different URLs → same `filepath.Base`) | Append content hash to filename on collision |
| Large images slow down S3 upload | Size limit (10MB, already in export), progress indicator |
| Andamio API adds native upload, making Phase 2 redundant | Phase 2 is optional; Phase 1 is useful regardless |

## Sources

- **Origin brainstorm:** [docs/brainstorms/2026-03-16-course-module-import-export-brainstorm.md](../brainstorms/2026-03-16-course-module-import-export-brainstorm.md) — key decision: "Image handling: Upload to Andamio CDN via API"
- **Current import:** `cmd/andamio/course_import.go` — image handling at lines 399-414 (block) and 566-591 (inline)
- **Current export:** `cmd/andamio/course_export.go` — image download at lines 482-498, 682-773
- **API schema:** `AggregateUpdateModuleV2Request` — `image_url` is a string field, no file upload
- **Previous plan:** [docs/plans/2026-03-16-feat-course-import-text-only-plan.md](2026-03-16-feat-course-import-text-only-plan.md) — deferred image upload as future work
