---
title: "test: Image Manifest Round-Trip Manual Testing"
type: test
status: active
date: 2026-03-16
---

# Manual Test: Image Manifest Round-Trip

## Prerequisites

```bash
go build -o andamio ./cmd/andamio
andamio user login
```

You need a course ID and a module code that has images in its lessons.

## 1. Export a module with images

```bash
andamio course export <course-id> <module-code>
```

- [ ] Export completes without errors
- [ ] `compiled/<slug>/<code>/assets/` directory contains downloaded images
- [ ] `compiled/<slug>/<code>/assets/.image-manifest.json` exists
- [ ] Manifest JSON maps each image filename to its CDN URL
- [ ] Lesson markdown files reference images as `![alt](assets/filename.png)`

## 2. Verify manifest content

```bash
cat compiled/<slug>/<code>/assets/.image-manifest.json | python3 -m json.tool
```

- [ ] Valid JSON
- [ ] Every image in `assets/` (except `.image-manifest.json`) has an entry
- [ ] All URLs start with `https://`

## 3. Import without changes (identity round-trip)

```bash
andamio course import compiled/<slug>/<code> --course-id <course-id>
```

- [ ] Output says `Found image manifest: N image(s) will use original URLs`
- [ ] No `Warning: N new image(s)` message
- [ ] Import completes successfully
- [ ] Check the course in the web UI — images still display correctly

## 4. Edit content and re-import

```bash
# Edit a lesson file (change some text, leave images alone)
vim compiled/<slug>/<code>/lesson-1.md
andamio course import compiled/<slug>/<code> --course-id <course-id>
```

- [ ] Manifest still used, images preserved
- [ ] Text changes appear in the web UI
- [ ] Images unchanged in web UI

## 5. Add a new image (not in manifest)

```bash
# Copy any PNG into assets
cp ~/some-image.png compiled/<slug>/<code>/assets/new-diagram.png
# Reference it in a lesson
echo '![new diagram](assets/new-diagram.png)' >> compiled/<slug>/<code>/lesson-1.md
andamio course import compiled/<slug>/<code> --course-id <course-id>
```

- [ ] Output warns: `Warning: 1 new image(s) in assets/ cannot be uploaded`
- [ ] Lists `new-diagram.png`
- [ ] Existing manifest images still resolve correctly
- [ ] New image appears as `[Image: new diagram]` placeholder in web UI

## 6. Import without manifest (backward compatibility)

```bash
rm compiled/<slug>/<code>/assets/.image-manifest.json
andamio course import compiled/<slug>/<code> --course-id <course-id>
```

- [ ] No `Found image manifest` message
- [ ] All local images warned as before
- [ ] Import still completes (no crash)

## 7. Corrupted manifest

```bash
echo "not json" > compiled/<slug>/<code>/assets/.image-manifest.json
andamio course import compiled/<slug>/<code> --course-id <course-id>
```

- [ ] Output warns: `Warning: could not parse image manifest`
- [ ] Falls back to placeholder behavior
- [ ] Import still completes
