---
status: complete
priority: p1
issue_id: "012"
tags: [code-review, composability, json, user-status]
dependencies: []
---

# `SessionExpired bool` with `omitempty` Drops Meaningful `false` State

## Problem Statement

`user.go` defines:

```go
SessionExpired bool `json:"session_expired,omitempty"`
```

`omitempty` on a `bool` omits the field when its value is `false`. But `session_expired: false` means "the session is valid" — that is a meaningful, informative state distinct from "no session information available." A script doing:

```bash
andamio user status --output json | jq '.session_expired // false'
```

will silently return `false` for an unauthenticated user (field absent → `// false` fallback), which is indistinguishable from an authenticated user with a valid session. This is a composability bug.

Additionally, when `JWTExpiresAt` is non-empty but contains a non-RFC3339 timestamp, `time.Parse` fails silently, `SessionExpired` stays `false` (zero value), and is omitted from output — leaving the consumer with no signal that expiry state is unknown.

**Impact:** Agents and scripts using `user status --output json` to gate re-authentication decisions will silently misclassify expired or unauthenticated sessions as valid.

## Findings

- **Source**: Code quality agent (P1), simplicity agent (Medium), agent-native reviewer (P1 #2)
- **Location**: `cmd/andamio/user.go:267`

## Proposed Solutions

### Option A: Remove `omitempty` from `SessionExpired` (Recommended for simplicity)

```go
SessionExpired bool `json:"session_expired"`
```

**Pros:** One-character change. Field always present when `UserAuthenticated` is true. Scripts can check `session_expired === false` with confidence.
**Cons:** When `UserAuthenticated` is false, `session_expired` will appear as `false` in the output (a meaningless default). But since `user_authenticated: false` already signals "no session", scripts should check that first.
**Effort:** Trivial
**Risk:** None

### Option B: Use `*bool` with `omitempty` (Recommended for precision)

```go
SessionExpired *bool `json:"session_expired,omitempty"`
```

Set to a `true`/`false` pointer only when `UserAuthenticated` is true. This makes the field absent for unauthenticated users (correct), present and `false` for valid sessions, and present and `true` for expired sessions. Follows the same pattern as `UserAlias` and `UserID` which are `omitempty` strings absent when unauthenticated.

```go
expired := now.After(expiresAt)
result.SessionExpired = &expired
```

**Pros:** Most precise representation. Distinguishes "not applicable" from "false".
**Cons:** Adds a pointer type. Callers must handle `null` in JSON.
**Effort:** Small
**Risk:** Low — only affects JSON output schema

### Option C: Move session fields to a nested object

```json
{
  "session": {
    "expired": false,
    "remaining_seconds": 12345
  }
}
```

**Pros:** Clean grouping. Presence/absence of the `session` key signals authentication state.
**Cons:** Breaking schema change. More work.
**Effort:** Medium
**Risk:** Medium (schema change)

## Recommended Action

Option B (`*bool` pointer). Consistent with how other optional fields are handled. Nil = not authenticated, `false` = valid, `true` = expired.

## Technical Details

- **Affected files**: `cmd/andamio/user.go:267`
- **PR**: #15 fix/composability-gaps

## Acceptance Criteria

- [ ] `andamio user status --output json | jq '.session_expired'` returns `false` (not `null`) for an authenticated user with a valid session
- [ ] `andamio user status --output json | jq '.session_expired'` returns `null` or is absent for an unauthenticated user
- [ ] `andamio user status --output json | jq '.session_expired'` returns `true` for an expired session
- [ ] All three states are unambiguously distinguishable

## Work Log

- 2026-03-18: Flagged by code-quality, simplicity, and agent-native review agents during PR #15 review
