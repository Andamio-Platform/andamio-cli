---
title: "Fix apikey auth header conflict and course content 404 UX"
date: 2026-03-18
problem_type: integration-issues
module: auth, course content discovery
symptoms:
  - apikey profile and apikey usage return 401 when wallet JWT is configured
  - Error message "Invalid or expired Developer JWT" provides no actionable guidance
  - course lesson/intro/assignment 404 errors give no hint about content availability
  - course slts displays raw JSON without lesson presence info
  - CSV and markdown output formats silently fall through to text table
root_cause: "HTTP client unconditionally sends both X-API-Key and Authorization headers; course commands return raw 404s without context"
severity: medium
tags:
  - auth
  - composability
  - error-messages
  - output-formats
  - utf8
  - course-content
---

# Fix apikey auth header conflict and course content 404 UX

> **⚠️ SUPERSEDED (Issue #17 portion only) — 2026-05-19.** The Issue #17 fix
> below (`getAPIKeyJSON` strips the JWT and sends *only* `X-API-Key`) was
> correct for the gateway behavior at the time: `/v2/apikey/developer/*`
> then *rejected* dev/wallet JWTs. The gateway has since moved that surface
> behind `developerJWTAuth` — it is now a **dual-credential** surface
> (`X-API-Key` **and** `Authorization: Bearer <devJWT>` both required),
> identical to `/v2/keys`. The strip-the-JWT approach now *causes* the very
> 401 it was written to prevent. `apikey usage`/`apikey profile` now route
> through `devKeysClient`; see CHANGELOG `[Unreleased]` and
> `cmd/andamio/apikey_test.go`. The Issue #18 (course content 404 UX)
> portion of this document is unaffected and still current.

## Problem Statement

### Issue #17: apikey commands fail with wallet JWT

When a developer authenticates both via `andamio auth login --api-key <key>` and `andamio user login` (wallet), the `apikey profile` and `apikey usage` commands fail with 401.

**Root cause**: The HTTP client's `setHeaders()` unconditionally sends both `X-API-Key` and `Authorization: Bearer <jwt>` headers when both credentials exist in config. The `/v2/apikey/developer/*` endpoints only accept API key auth and reject the wallet JWT, returning "Invalid or expired Developer JWT."

```bash
andamio user login        # authenticate via wallet
andamio user status       # confirms session active
andamio apikey profile    # → 401 "Invalid or expired Developer JWT"
```

### Issue #18: course content 404 UX lacks guidance

`course lesson`, `course intro`, and `course assignment` return raw API 404 errors with no context about why or how to discover valid content. Users cannot tell whether the course, module, SLT, or lesson content is missing.

Additionally, `course slts` returned raw JSON without showing which SLTs have associated lesson content, forcing trial-and-error discovery.

## Solution

### Fix #17: Config-copy-before-mutation pattern

Created `getAPIKeyJSON()` helper that copies the config struct, clears the JWT on the copy, and creates a client that only sends `X-API-Key`:

```go
func getAPIKeyJSON(path string) error {
    cfg, err := config.Load()
    if err != nil {
        return err
    }
    if cfg.APIKey == "" {
        return &apierr.AuthError{
            Message: "apikey commands require an API key. Run 'andamio auth login --api-key <key>'",
        }
    }
    // Copy config and clear wallet JWT so only X-API-Key is sent
    apiKeyCfg := *cfg
    apiKeyCfg.UserJWT = ""
    c := client.New(&apiKeyCfg)
    var result map[string]interface{}
    if err := c.Get(path, &result); err != nil {
        return err
    }
    return output.PrintJSON(result)
}
```

**Key decision**: Copy the struct (`apiKeyCfg := *cfg`) rather than mutating the pointer. Prevents unintended side effects if config is used again later or if a `Save()` call is ever added downstream.

### Fix #18: `getJSONWithHint` pattern for contextual 404s

Extracted a reusable helper that intercepts `NotFoundError` and replaces the message with actionable guidance:

```go
func getJSONWithHint(path, notFoundHint string) error {
    err := getJSON(path)
    if err != nil {
        var notFound *apierr.NotFoundError
        if errors.As(err, &notFound) {
            return &apierr.NotFoundError{Message: notFoundHint}
        }
        return err
    }
    return nil
}
```

Each command provides a hint referencing the appropriate discovery command:

```go
// courseLessonCmd
hint := fmt.Sprintf("No lesson found for SLT %s in module %s. "+
    "Run 'andamio course slts %s %s' to see which SLTs have lessons.",
    sltIndex, moduleCode, courseID, moduleCode)
return getJSONWithHint(path, hint)
```

### Fix #18B: Teacher/user endpoint fallback for `course slts`

Refactored `course slts` to show a formatted table with lesson presence via the teacher endpoint:

```
INDEX   SLT TEXT                                           HAS LESSON
-----   --------                                           ----------
2       I know enough about Cardano to start this course.  Yes
3       I understand the essentials of how Ouroboros wo... Yes
7       Should we introduce Cardano Up here?               No
```

Uses `cfg.HasUserAuth()` to choose the teacher endpoint (with lesson data) when JWT is available, falling back to the user endpoint for API-key-only users.

## Review Findings Fixed

The code review caught four additional issues:

### P1: CSV/markdown format bug

`runCourseSltsTeacher` only checked for `FormatJSON`, silently falling through to text table for CSV and markdown. Fixed by building structured items once and routing non-text formats through `output.PrintJSON`:

```go
// Before (broken): only handled JSON, text got everything else
if output.GetFormat() == output.FormatJSON { ... }

// After (fixed): text gets custom table, all others get structured data
if output.GetFormat() != output.FormatText {
    return output.PrintJSON(map[string]interface{}{"data": items})
}
```

### P2: UTF-8 truncation

Byte-slicing (`text[:45]`) breaks multi-byte UTF-8 characters. Replaced with rune-based helper:

```go
func truncateUTF8(s string, maxRunes int) string {
    if utf8.RuneCountInString(s) <= maxRunes {
        return s
    }
    runes := []rune(s)
    return string(runes[:maxRunes-3]) + "..."
}
```

### P2: Config pointer mutation

Changed from `cfg.UserJWT = ""` (mutates shared pointer) to `apiKeyCfg := *cfg` (copy-then-modify).

### P2: Boilerplate extraction

Extracted `getJSONWithHint` from three copy-pasted 12-line error-wrapping blocks, reducing each command to a one-liner.

## Reusable Patterns

| Pattern | When to Use | Example |
|---------|-------------|---------|
| Config-copy-before-mutation | Modifying loaded config for a specific request | `apiKeyCfg := *cfg; apiKeyCfg.UserJWT = ""` |
| `getJSONWithHint` | Any GET that can 404 where guidance helps | `getJSONWithHint(path, "Run 'andamio X' to discover...")` |
| Teacher/user endpoint fallback | Command benefits from richer data when JWT available | `if cfg.HasUserAuth() { return teacherPath() }` |
| `truncateUTF8` | Any table output with user-supplied text | `text = truncateUTF8(text, 50)` |
| Structured items for multi-format | Commands with custom table output | Build `[]map[string]interface{}`, dispatch on format |

## Prevention Checklist

When adding new CLI commands, verify:

- [ ] All output formats handled (text, json, csv, markdown) — not just json
- [ ] 404s include contextual hints referencing discovery commands
- [ ] Auth errors use `*apierr.AuthError` (exit code 3), not generic errors
- [ ] Config is copied before mutation, never modified in place
- [ ] String truncation uses `truncateUTF8`, not byte slicing
- [ ] Endpoints assessed for auth header conflicts (API-key-only vs dual-auth)
- [ ] Progress messages go to stderr, structured data to stdout only

## Related Documentation

- `docs/solutions/integration-issues/cli-api-auth-middleware-mismatch.md` — v1/v2 auth header conflict pattern
- `docs/solutions/architecture/cli-composability-audit-and-fix.md` — exit code contract and typed error hierarchy
- `docs/solutions/security-issues/cli-security-hardening-input-validation.md` — URL path escaping, credential masking
- `docs/solutions/architecture/command-structure-refactoring.md` — `getJSON`/`printList` helper patterns
- `docs/brainstorms/2026-03-18-composability-fixes-brainstorm.md` — scripting contract decisions
- GitHub Issues: [#17](https://github.com/Andamio-Platform/andamio-cli/issues/17), [#18](https://github.com/Andamio-Platform/andamio-cli/issues/18)

## Files Modified

| File | Changes |
|------|---------|
| `cmd/andamio/apikey.go` | Added `getAPIKeyJSON()` with config-copy pattern |
| `cmd/andamio/course.go` | Added `getJSONWithHint()`, `truncateUTF8()`, `runCourseSlts()`, `runCourseSltsTeacher()` |
