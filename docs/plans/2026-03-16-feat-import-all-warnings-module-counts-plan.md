---
title: "feat: import-all, H1 warnings, and module counts"
type: feat
status: completed
date: 2026-03-16
---

# feat: import-all, H1 warnings, and module counts

Three small features that improve the CLI's usability for batch workflows and content validation.

## Issue #13: Warn when lesson file has no H1 title heading (Small)

### Problem
Lesson files without `# H1` heading import with blank titles. No warning is shown — the blank title only appears in the app UI.

### Acceptance Criteria
- [x] Warning emitted during import when a lesson file has no H1: `Warning: lesson-1.md has no # title heading`
- [x] Warning also emitted for introduction.md and assignment.md with no H1
- [x] Warnings suppressed in `--output json` mode (consistent with existing pattern)
- [x] Import still succeeds (warning, not error)

### Implementation
In `readCompiledModule`, after `extractH1Title` returns, check if title is empty:

```go
// cmd/andamio/course_import.go — in the lesson reading loop (~line 400)
title, body := extractH1Title(string(content))
if title == "" && output.GetFormat() != output.FormatJSON {
    fmt.Printf("Warning: %s has no # title heading — lesson will import without a title\n", filepath.Base(lessonFile))
}
```

Same pattern for introduction.md and assignment.md blocks below.

---

## Issue #12: import-all command for batch module import (Medium)

### Problem
Importing a full course requires running `andamio course import` for each module directory manually.

### Acceptance Criteria
- [x] `andamio course import-all <dir> --course-id <id>` imports all subdirectories containing `outline.md`
- [x] Modules sorted by directory name (numeric sort: 101, 102, 103)
- [x] Summary printed after all imports
- [x] `--create` flag passed through to each module import (creates missing modules)
- [x] `--dry-run` flag shows what would be imported without sending
- [x] `--continue-on-error` flag continues past failures (default: stop on first error)
- [x] JSON output returns array of per-module results

### Implementation
New file `cmd/andamio/course_import_all.go`:

1. Scan `dir/*/outline.md` to find module subdirectories
2. Sort numerically by directory name
3. For each directory, call the core import logic (extract a shared function from `runCourseImport`)
4. Collect results, print summary table

Key design: Extract the import logic into a reusable function rather than shelling out to `runCourseImport`. Something like:

```go
func importModule(c *client.Client, cfg *config.Config, moduleDir, courseID string, opts ImportOptions) (*ImportResult, error)
```

Where `ImportOptions` holds `DryRun`, `CreateMode`, `SortOrder`. Both `runCourseImport` and `runImportAll` call this.

### Flags
- `--course-id <id>` (required)
- `--create` — pass through to per-module import
- `--dry-run` — show plan without sending
- `--continue-on-error` — don't stop on first failure
- `--sort-order-start <n>` — starting sort order for `--create` (increments per module)

---

## Issue #14: course modules should show SLT/lesson counts (Small-Medium)

### Problem
`andamio course modules <course-id>` uses the user endpoint which returns minimal data. No way to verify content counts after import.

### Acceptance Criteria
- [x] New `andamio course modules <course-id> --teacher` flag (or separate `teacher modules` subcommand) that uses the teacher endpoint
- [x] Table output shows: Module Code, Title, Status, SLTs, Lessons, Assignment
- [x] Lesson count derived from counting `slt.lesson` entries in the teacher response
- [x] JSON output includes full data

### Implementation
The teacher endpoint `POST /v2/course/teacher/course-modules/list` already returns full SLT and lesson data (used by `fetchExistingModule`). Add a `--teacher` flag to `courseModulesCmd` that:

1. Uses `POST /v2/course/teacher/course-modules/list` instead of the user GET endpoint
2. Extracts: module code, title, status, SLT count, lesson count (count non-nil `slt.lesson` entries), assignment presence
3. Prints as a formatted table in text mode

```go
// cmd/andamio/course.go or new file
// In the teacher path:
for _, mod := range modules {
    content := mod["content"]
    slts := content["slts"].([]interface{})
    sltCount := len(slts)
    lessonCount := 0
    for _, slt := range slts {
        if slt["lesson"] != nil { lessonCount++ }
    }
    hasAssignment := content["assignment"] != nil
}
```

### Note
The user endpoint may not return enough data. If `--teacher` feels clunky, we could make it the default when the user has teacher auth (check `cfg.HasUserAuth()`), falling back to the user endpoint otherwise.

---

## Priority Order

1. **#13** (Small, 15 min) — trivial warning addition, immediate value
2. **#14** (Small-Medium, 30 min) — teacher flag on modules command
3. **#12** (Medium, 1 hr) — requires extracting shared import logic

## Sources

- [#12](https://github.com/Andamio-Platform/andamio-cli/issues/12) — import-all
- [#13](https://github.com/Andamio-Platform/andamio-cli/issues/13) — H1 warning
- [#14](https://github.com/Andamio-Platform/andamio-cli/issues/14) — module counts
- `cmd/andamio/course_import.go:395-410` — lesson H1 extraction
- `cmd/andamio/course.go:35-42` — current modules command (user endpoint)
- `cmd/andamio/course_import.go:955-1023` — fetchExistingModule (teacher endpoint parsing)
