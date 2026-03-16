---
title: "CLI Image Upload via App /api/upload Endpoint"
date: 2026-03-16
category: feature-implementations
tags:
  - course-import
  - image-upload
  - gcs
  - multipart
  - cdn
components:
  - cmd/andamio/course_import.go
symptoms:
  - "New images in assets/ become [Image: alt] placeholders after import"
  - "Upload failed (400): Invalid file type. Allowed: PNG, JPG, GIF, WebP"
root_cause: "CLI had no image upload capability; multipart Content-Type defaulted to application/octet-stream"
severity: medium
related_docs:
  - docs/solutions/feature-implementations/cli-course-import-image-manifest-roundtrip.md
  - docs/solutions/integration-issues/cli-course-import-app-parity-and-payload-alignment.md
---

# CLI Image Upload via App /api/upload Endpoint

## Problem

New images added to the `assets/` directory (not in the image manifest from a previous export) could not be uploaded. They became `[Image: alt]` placeholder text after import, because the CLI had no way to get a CDN URL for them.

## Solution: Use the App's Upload Endpoint

The Andamio web app exposes `POST /api/upload` which accepts multipart file uploads and returns a public GCS URL. The CLI already has the user's JWT from `andamio user login`, and derives the app URL from the API URL (same pattern used for OAuth login).

### Upload Flow

```
CLI detects new image (not in manifest)
  → POST {appURL}/api/upload (multipart/form-data, Bearer JWT)
  → App validates type/size, uploads to GCS
  → Returns: {"url": "https://storage.googleapis.com/bucket/uploads/uuid.png"}
  → CLI adds URL to manifest
  → Writes updated manifest to disk
  → Re-reads module (resolveManifestPaths picks up new URLs)
  → Import proceeds with full CDN URLs
```

### Key Implementation Details

**App URL derivation** (same as OAuth flow in `user.go`):
```go
appURL := strings.Replace(cfg.BaseURL, ".api.", ".app.", 1)
uploadURL := appURL + "/api/upload"
```

**Multipart form with correct MIME type** — the initial attempt failed because `CreateFormFile` defaults to `application/octet-stream`. The server validates MIME type from the multipart header, not file contents:

```go
// WRONG: CreateFormFile sets Content-Type: application/octet-stream
part, _ := writer.CreateFormFile("file", filename)

// CORRECT: CreatePart with explicit MIME type
partHeader := make(textproto.MIMEHeader)
partHeader.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="%s"`, filename))
partHeader.Set("Content-Type", mimeType) // e.g., "image/png"
part, _ := writer.CreatePart(partHeader)
```

**MIME type from extension:**
| Extension | MIME Type |
|-----------|-----------|
| `.png` | `image/png` |
| `.jpg`, `.jpeg` | `image/jpeg` |
| `.gif` | `image/gif` |
| `.webp` | `image/webp` |

**Validation (matches app):**
- Max 5MB per file
- Allowed: PNG, JPEG, GIF, WebP (no SVG)
- JWT required

### Manifest Update Workflow

After uploading, the CLI writes the updated manifest to disk before re-reading the module:

```go
// Upload returns CDN URL → add to in-memory manifest
manifest[filename] = cdnURL

// Write to disk so re-read picks it up
manifestData, _ := json.MarshalIndent(manifest, "", "  ")
os.WriteFile(filepath.Join(assetsDir, ".image-manifest.json"), manifestData, 0644)

// Re-read module — resolveManifestPaths now resolves the new image
data, _ = readCompiledModule(moduleDir)
```

This means the manifest file is permanently updated. Future imports of the same module won't need to re-upload the image.

### Response Format

```json
{
  "url": "https://storage.googleapis.com/andamio-v2-preprod-uploads/uploads/80c80df4-70e2-4d19-99d2-5f45b4a0b5d9.png",
  "key": "uploads/80c80df4-70e2-4d19-99d2-5f45b4a0b5d9.png",
  "size": 89370,
  "contentType": "image/png"
}
```

The URL is public and permanent (1-year cache header, UUID-based filename).

## Prevention

- Always set the correct `Content-Type` on multipart form parts — don't rely on `CreateFormFile` defaults
- Validate file type client-side before uploading (reject unsupported extensions early with a clear message)
- The manifest-on-disk pattern ensures uploads are idempotent — re-running import doesn't re-upload

## Testing

- [ ] Upload a PNG image — verify CDN URL returned and image accessible
- [ ] Upload a JPEG image — verify correct MIME type sent
- [ ] Upload a file > 5MB — verify rejection with clear error
- [ ] Upload an SVG — verify rejection before sending
- [ ] Upload without JWT — verify 401 error
- [ ] Re-import after upload — verify manifest prevents re-upload
- [ ] Verify uploaded image renders in web app after import
