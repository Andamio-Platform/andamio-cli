---
title: "Fix apikey auth header conflict and course lesson 404 UX"
type: fix
status: completed
date: 2026-03-18
---

# Fix apikey auth header conflict and course lesson 404 UX

## Overview

Two UX bugs that erode trust in the CLI's composability contract:

1. **#17 — apikey commands fail with wallet JWT**: `apikey profile` and `apikey usage` return 401 because `setHeaders()` sends both `X-API-Key` and `Authorization: Bearer` headers. The API prioritizes the wallet JWT and rejects it as an invalid "Developer JWT." The commands are unreachable for users who authenticated via `user login`.

2. **#18 — course lesson returns confusing 404**: `course lesson` returns a raw API 404 when the SLT exists but has no lesson content. Users can't discover which SLTs have lessons without trial-and-error.

## Problem Statement

**#17**: The HTTP client unconditionally sends both auth headers when both credentials exist in config (`client.go:76-84`). The `/v2/apikey/developer/*` endpoints only accept API key auth, but the wallet JWT in the `Authorization` header causes the API to reject the request. The error message "Invalid or expired Developer JWT" leaks an API implementation detail and gives the user no actionable guidance.

**#18**: The `course lesson`, `course intro`, and `course assignment` commands use `getJSON()` which returns raw API errors. A 404 could mean the course doesn't exist, the module doesn't exist, the SLT doesn't exist, or the SLT simply has no lesson content. The user gets no guidance on which case they hit or how to discover valid targets.

## Proposed Solution

### #17 — apikey commands: API-key-only client

Replace the bare `getJSON()` calls in `apikey.go` with a dedicated helper that:

1. Loads config and validates `cfg.APIKey != ""`
2. Creates a client with `UserJWT` cleared so only `X-API-Key` is sent
3. Returns a clear error if no API key is configured

```go
// cmd/andamio/apikey.go

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
	// Clear wallet JWT so only X-API-Key is sent
	cfg.UserJWT = ""
	c := client.New(cfg)
	var result map[string]interface{}
	if err := c.Get(path, &result); err != nil {
		return err
	}
	return output.PrintJSON(result)
}
```

Then update both commands:

```go
// apikey usage
RunE: func(cmd *cobra.Command, args []string) error {
    return getAPIKeyJSON("/api/v2/apikey/developer/usage/get")
},

// apikey profile
RunE: func(cmd *cobra.Command, args []string) error {
    return getAPIKeyJSON("/api/v2/apikey/developer/profile/get")
},
```

**Why this approach over alternatives:**
- Option 1 (developer auth flow) adds significant new surface area for an unclear user base — deferred
- Option 2 (API change) pushes complexity to the API team and the wallet→developer-account mapping is ambiguous
- Option 3 (this approach) is ~20 lines, zero architectural risk, and unblocks the commands for their intended audience (developers with API keys)

### #18 — course lesson: contextual 404 + SLT discovery

**Part A — Contextual 404 messages** for `course lesson`, `course intro`, and `course assignment`:

```go
// cmd/andamio/course.go — courseLessonCmd

RunE: func(cmd *cobra.Command, args []string) error {
    courseID, moduleCode, sltIndex := args[0], args[1], args[2]
    err := getJSON(fmt.Sprintf("/api/v2/course/user/lesson/%s/%s/%s",
        url.PathEscape(courseID), url.PathEscape(moduleCode), url.PathEscape(sltIndex)))
    if err != nil {
        var notFound *apierr.NotFoundError
        if errors.As(err, &notFound) {
            return &apierr.NotFoundError{
                Message: fmt.Sprintf("No lesson found for SLT %s in module %s. "+
                    "Run 'andamio course slts %s %s' to see which SLTs have lessons.",
                    sltIndex, moduleCode, courseID, moduleCode),
            }
        }
        return err
    }
    return nil
},
```

Apply the same pattern to `courseIntroCmd` and `courseAssignmentCmd` (without SLT index, since those are per-module).

**Part B — Surface lesson presence in `course slts` output:**

Refactor `course slts` from a raw `getJSON()` dump to a formatted table. Check whether the API response includes a `has_lesson` field or nested `lesson` object. If present, show it:

```
INDEX  SLT TEXT                              HAS LESSON
1      Explain the UTXO model                No
2      Build a simple transaction             Yes
3      Describe native assets on Cardano      Yes
```

If the user endpoint doesn't include lesson presence data, fall back to the teacher endpoint when JWT is available (same conditional pattern used in `runCourseModules` at `course.go:161-176`).

## Acceptance Criteria

### #17 — apikey auth fix

- [x] `andamio apikey profile` works when only API key is configured (no wallet JWT)
- [x] `andamio apikey usage` works when only API key is configured
- [x] Both commands work when *both* API key and wallet JWT are configured (wallet JWT not sent)
- [x] Clear error message when no API key is configured: tells user to run `auth login --api-key`
- [x] Error returns `AuthError` type (exit code 3)
- [x] `--output json` produces `{"error": "..."}` envelope on auth failure
- [ ] Uses `url.PathEscape()` for any user-supplied path segments (per security learnings) — N/A: apikey endpoints have no user-supplied path segments

### #18 — course lesson 404 UX

- [x] `course lesson` 404 shows contextual message with guidance to run `course slts`
- [x] `course intro` 404 shows contextual message referencing `course modules`
- [x] `course assignment` 404 shows contextual message referencing `course modules`
- [x] `course slts` displays formatted table in text mode (not raw JSON dump)
- [x] `course slts` shows lesson presence per SLT (via teacher endpoint when JWT available)
- [x] `--output json` mode still returns raw API response for `course slts`
- [x] Exit code 2 preserved for not-found errors
- [ ] `--output json` error envelope includes hint field for scriptability — deferred: would require changes to main.go error handling

## Dependencies & Risks

**Verify before implementing:**

1. **API header priority**: Does the API always prioritize `Authorization` over `X-API-Key` when both are sent? If yes, the `apikey.go` fix is correct. If no (API uses `X-API-Key` when present), the bug is elsewhere.
2. **`course/user/slts` response shape**: Does it include `has_lesson` or a nested `lesson` object? This determines whether Part B of #18 needs a teacher endpoint fallback.

**Low risk**: Both fixes are isolated to command handlers. No changes to `client.go`, `config.go`, or the error type system. The `getAPIKeyJSON` helper is self-contained.

## Implementation Notes

### Files to modify

| File | Change |
|------|--------|
| `cmd/andamio/apikey.go` | Add `getAPIKeyJSON()` helper, update both command `RunE` functions |
| `cmd/andamio/course.go` | Wrap 404 in `courseLessonCmd`, `courseIntroCmd`, `courseAssignmentCmd`; refactor `courseSltsCmd` to use formatted table |

### Institutional learnings to apply

- **URL path escaping** (`docs/solutions/security-issues/cli-security-hardening-input-validation.md`): Use `url.PathEscape()` for all user-supplied path segments
- **Auth error types** (`docs/solutions/architecture/cli-composability-audit-and-fix.md`): Return `*apierr.AuthError` from local auth checks to maintain exit code 3 contract
- **JSON error output** (`docs/solutions/architecture/cli-composability-audit-and-fix.md`): Use `json.Marshal()` for error envelopes, never `fmt.Sprintf`

### Scope boundary

- Do NOT add developer auth flow — that's a separate feature if needed later
- Do NOT modify `client.go` header logic globally — the per-command approach is safer
- Do NOT change exit codes — stay within the existing 0/1/2/3 contract

## Sources

- GitHub Issue: #17 — apikey commands fail with wallet JWT
- GitHub Issue: #18 — course lesson confusing 404
- Composability brainstorm: `docs/brainstorms/2026-03-18-composability-fixes-brainstorm.md` — exit code contract referenced
- Learning: `docs/solutions/architecture/cli-composability-audit-and-fix.md` — typed error hierarchy
- Learning: `docs/solutions/integration-issues/cli-api-auth-middleware-mismatch.md` — header conflict pattern
- Learning: `docs/solutions/security-issues/cli-security-hardening-input-validation.md` — URL path escaping
