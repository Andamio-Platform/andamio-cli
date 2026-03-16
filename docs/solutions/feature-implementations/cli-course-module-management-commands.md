---
title: "CLI course module management: create-module, import --create, import-all, content counts, and shared importModule refactor"
date: 2026-03-16
category: feature-implementations
tags:
  - go-cli
  - cobra
  - course-management
  - import-export
  - refactoring
  - error-handling
  - batch-operations
  - sentinel-errors
  - teacher-endpoint
components:
  - cmd/andamio/course.go
  - cmd/andamio/course_create_module.go
  - cmd/andamio/course_import.go
  - cmd/andamio/course_import_all.go
symptoms:
  - "New modules failed SLT creation when slt_index was included in payload"
  - "No CLI command to create modules — required manual API calls"
  - "No batch import for multiple modules"
  - "Lesson files without H1 title imported silently with blank titles"
  - "course modules listing lacked content counts"
  - "Duplicate import logic between import and import-all caused FailedImages bug"
root_cause: "Multiple missing features and API contract misunderstandings. Code duplication between import and import-all created divergence bugs."
severity: medium
time_to_resolve: "4 hours"
---

# CLI Course Module Management Commands

## Overview

Implementation of six features (#9-#14) that complete the CLI's course module management story: creating modules, batch importing, content validation warnings, and rich module listings. Also includes an architectural refactor that extracted shared import logic into a reusable `importModule()` function.

## Features Implemented

### 1. SLT Creation for New Modules (#9)

**Problem:** Import always sent `slt_index` in the SLT payload. The API interprets this as "update existing SLT at this index" — which silently does nothing on a new module with no SLTs.

**Solution:** Track `SLTCount` in `ExistingModuleData`. Omit `slt_index` when `SLTCount == 0` to trigger creation:

```go
slt := map[string]interface{}{"slt_text": sltText}
if existing.SLTCount > 0 {
    slt["slt_index"] = i + 1  // update existing
}
// else: omit slt_index → API creates new SLT
```

### 2. create-module Command (#10)

New standalone command that creates a module shell via the teacher API:

```bash
# From outline.md
andamio course create-module ./compiled/my-course/101 --course-id <id>

# With explicit flags
andamio course create-module --course-id <id> --code 101 --title "My Module"
```

Uses `readOutlineMetadata()` — a lightweight parser that only reads frontmatter, not lesson content. This avoids failing on directories with incomplete lesson files.

### 3. import --create Flag (#11)

Auto-creates missing modules during import:

```bash
andamio course import ./module --course-id <id> --create --sort-order 1
```

Uses sentinel error `errModuleNotFound` to distinguish "module doesn't exist" from auth/network failures:

```go
var errModuleNotFound = errors.New("module not found")

// In fetchExistingModule:
return nil, fmt.Errorf("%w: '%s' in course '%s'", errModuleNotFound, moduleCode, courseID)

// In runCourseImport:
if createMode && errors.Is(err, errModuleNotFound) {
    // create module, then re-fetch
} else {
    return err  // surface auth/network errors directly
}
```

### 4. import-all Batch Command (#12)

Imports all module subdirectories containing `outline.md`:

```bash
andamio course import-all ./compiled/my-course --course-id <id> --create --continue-on-error
```

Features:
- Numeric directory sort (101, 102, 103)
- `--create` creates missing modules with auto-incrementing `--sort-order-start`
- `--continue-on-error` continues past failures
- Summary table after completion

### 5. H1 Title Warnings (#13)

Warns when lesson/intro/assignment files lack a `# Heading`:

```
Warning: lesson-1.md has no # title heading — lesson will import without a title
```

Warnings suppressed in `--output json` mode.

### 6. Module Content Counts (#14)

`course modules` now auto-detects teacher auth and uses the richer teacher endpoint:

```
Code     Title                                    Status       SLTs Lessons Assignment
----     -----                                    ------       ---- ------- ----------
101      Your On-Chain Identity                   DRAFT           3       3        Yes
102      Browsing Courses and Projects             DRAFT           3       3        Yes
```

Falls back to user endpoint when only API key auth is available.

## Architecture Decisions

### ImportParams Pattern

Extracted shared `importModule(ImportParams)` function to eliminate duplication between `course import` and `course import-all`:

```go
type ImportParams struct {
    Client     *client.Client
    Config     *config.Config
    ModuleDir  string
    CourseID   string
    CreateMode bool
    DryRun     bool
    SortOrder  int
    Quiet      bool
}

func importModule(p ImportParams) (*ImportResult, error) {
    // Single copy of: read → upload images → fetch/create → update → build result
}
```

**Why:** The initial implementation duplicated the 5-step orchestration (~80 lines) between `runCourseImport` and `importSingleModule`. The copies already diverged — `FailedImages` was set in one but not the other. The params struct also eliminates positional-argument swapping risk (8 string/bool parameters).

### Sentinel Errors

`errModuleNotFound` distinguishes "not found" from auth/network failures. Without this, an expired JWT + `--create` flag would attempt to create a module instead of surfacing the auth error.

### readOutlineMetadata

Lightweight parser that reads only `outline.md` frontmatter (title + code). Used by `create-module` to avoid parsing all lesson files just to get two YAML fields. A lesson file with a conversion error won't block module creation.

### Teacher Endpoint Auto-Detection

When `cfg.HasUserAuth()` is true, the CLI prefers the teacher endpoint which returns full content (SLTs, lessons, assignment data). This follows the principle: use the most specific endpoint available for the user's role.

## Prevention Strategies

### 1. Extract before duplicating
When adding a command that overlaps with an existing one, extract shared logic into a function first. Never copy-paste orchestration between command files.

### 2. Sentinel errors for error classification
Define typed errors for distinct failure classes (not found, unauthorized, server error). Callers should use `errors.Is()` to decide behavior, not string matching.

### 3. Lightweight parsers alongside heavy ones
If a function reads an entire directory but some callers only need 2 fields, provide a lightweight alternative. Heavy parsers create false dependencies.

### 4. Prefer richer endpoints
When the user has role-specific auth (teacher, manager), auto-detect and use the richer endpoint. Fall back to generic endpoints only when role auth is unavailable.

### 5. Sanitize strings at format boundaries
Any string embedded in a structured format (HTTP headers, markdown, YAML) must be sanitized for that format. Filenames in `Content-Disposition` need quote escaping. Titles in markdown need newline stripping.

## Related Documentation

- [Export/Import Round-Trip Title Preservation](../logic-errors/export-import-round-trip-title-preservation.md) — title extraction, SLT matching, API contract semantics
- [CLI Course Import: App Parity](../integration-issues/cli-course-import-app-parity-and-payload-alignment.md) — payload structure, SLT locking, metadata preservation
- [CLI Course Import: Markdown-to-Tiptap](./cli-course-import-markdown-to-tiptap-conversion.md) — goldmark parsing, tiptap conversion
- [CLI Course Export: Tiptap Conversion](./cli-course-export-tiptap-conversion.md) — reverse conversion, image download
- [Image Manifest Round-Trip](./cli-course-import-image-manifest-roundtrip.md) — manifest mechanism, URL preservation
- [Goldmark AST Walker Strategies](../architecture/goldmark-ast-walker-prevention-strategies.md) — block vs inline patterns

## GitHub Issues

- [#9](https://github.com/Andamio-Platform/andamio-cli/issues/9) — SLT creation fix (closed)
- [#10](https://github.com/Andamio-Platform/andamio-cli/issues/10) — create-module command (closed)
- [#11](https://github.com/Andamio-Platform/andamio-cli/issues/11) — import --create flag (closed)
- [#12](https://github.com/Andamio-Platform/andamio-cli/issues/12) — import-all batch command (closed)
- [#13](https://github.com/Andamio-Platform/andamio-cli/issues/13) — H1 title warnings (closed)
- [#14](https://github.com/Andamio-Platform/andamio-cli/issues/14) — module content counts (closed)
- [API #239](https://github.com/Andamio-Platform/andamio-api/issues/239) — lesson upsert for ON_CHAIN modules (fixed)
