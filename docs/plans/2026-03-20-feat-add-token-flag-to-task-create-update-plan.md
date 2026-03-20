---
title: "Add --token flag to project task create and update"
type: feat
status: completed
date: 2026-03-20
origin: docs/brainstorms/2026-03-18-project-task-management-brainstorm.md
---

# Add --token flag to project task create and update

## Overview

`andamio project task create` cannot assign native Cardano assets (e.g. XP tokens) to a task. The API and task import already support tokens — the CLI create/update commands just need the flag wired up.

## Problem Statement / Motivation

Creating XP tasks via CLI requires native asset assignment. Current workaround is task import with Markdown files, but inline `task create` and `task update` should support the same features. Discovered during CF demo prep (Mar 20).

## Proposed Solution

Add a repeatable `--token` flag to both `project task create` and `project task update`.

**Usage:**

```bash
# Single token
andamio project task create <project-id> \
  --title "Build a dApp" \
  --lovelace 5000000 \
  --expiration 2026-06-01 \
  --token "722c475bebb106799b109fc95301c9b796e1a37b6afc601359d54a04,XP,50"

# Multiple tokens
andamio project task create <project-id> \
  --title "Complete milestone" \
  --lovelace 5000000 \
  --expiration 2026-06-01 \
  --token "abc123...,XP,50" \
  --token "def456...,RewardToken,100"

# Update: add tokens to existing task
andamio project task update 3 --project-id <id> \
  --token "abc123...,XP,50"
```

**Format:** `--token "policy_id,asset_name,quantity"` — comma-separated triple, repeatable.

## Technical Approach

### File to modify

`cmd/andamio/project_task.go` — single file change.

### Existing patterns to follow

| Pattern | Source | Reference |
|---------|--------|-----------|
| `TaskToken` struct | `project_task_import.go:53-57` | Reuse directly (same package) |
| `StringArray` repeatable flag | `tx_submit.go:36` (`--submit-header`) | Use `StringArray` not `StringSlice` |
| Token payload inclusion | `project_task_import.go:249-251` | `payload["tokens"] = tokens` |
| Conditional update field | `project_task.go:474-501` | `cmd.Flags().Changed("token")` guard |

### Implementation steps

#### 1. Register `--token` flag in `init()` (~line 115)

Add to both `projectTaskCreateCmd` and `projectTaskUpdateCmd`:

```go
projectTaskCreateCmd.Flags().StringArray("token", nil,
    `Native asset token (repeatable, format: "policy_id,asset_name,quantity")`)
```

```go
projectTaskUpdateCmd.Flags().StringArray("token", nil,
    `Native asset token (repeatable, format: "policy_id,asset_name,quantity")`)
```

#### 2. Add `parseTokenFlags` helper function

Parse `[]string` from the flag into `[]TaskToken`. For each value:

1. Split on `,` — expect exactly 3 fields
2. `strings.TrimSpace()` each field (users will add spaces after commas)
3. Validate: policy_id non-empty, quantity is a valid non-negative integer string
4. Allow empty asset_name (valid for Cardano policy-only tokens: `"policy_id,,quantity"`)
5. Reject duplicate policy_id + asset_name combinations
6. Return `[]TaskToken` or error with clear message showing expected format

#### 3. Add tokens to create payload (~line 386)

After the existing `content_json` block in `runTaskCreate`:

```go
tokenStrs, _ := cmd.Flags().GetStringArray("token")
if len(tokenStrs) > 0 {
    tokens, err := parseTokenFlags(tokenStrs)
    if err != nil {
        return err
    }
    payload["tokens"] = tokens
}
```

#### 4. Add tokens to update payload (~line 501)

Using the existing `Changed()` guard pattern in `runTaskUpdate`:

```go
if cmd.Flags().Changed("token") {
    tokenStrs, _ := cmd.Flags().GetStringArray("token")
    tokens, err := parseTokenFlags(tokenStrs)
    if err != nil {
        return err
    }
    payload["tokens"] = tokens
}
```

## Design Decisions

### Delimiter: comma

Comma-separated triple (`policy_id,asset_name,quantity`). Policy IDs are 56-char hex, asset names are hex-encoded, quantities are integers — none contain commas. Matches the format specified in the issue.

### Flag type: `StringArray` (not `StringSlice`)

`StringArray` treats each `--token value` as one element without comma-splitting the value itself. This is critical since the value already contains commas as delimiters. Matches the established pattern from `--submit-header` in `tx_submit.go`.

### Update semantics: replace (not append)

When `--token` is provided on update, it replaces the entire token array. This is consistent with how SLT and lesson arrays work in course module updates (see brainstorm: docs/brainstorms/2026-03-18-project-task-management-brainstorm.md). Omitting `--token` on update leaves tokens unchanged (via `Changed()` guard).

**Limitation:** No way to clear all tokens in v1. Acceptable for now — can add `--clear-tokens` flag later if needed.

### Whitespace trimming: yes

`strings.TrimSpace()` on all three fields. Users will naturally type `"policy_id, asset_name, 50"`. Untrimmed spaces in hex policy IDs would cause silent Cardano-layer failures.

### Empty asset names: allowed

`"policy_id,,quantity"` is valid — Cardano policy-only tokens have empty asset names. Parser must not reject empty middle field.

### Quantity validation: CLI-side

Validate quantity is a non-negative integer string before sending to API. Early validation produces better error messages than API-level rejection.

### Duplicate detection: reject

If the same `policy_id + asset_name` pair appears twice, fail with a clear error. Prevents ambiguous API behavior.

## Error Messages

```
Error: invalid --token format "bad": expected "policy_id,asset_name,quantity"
  Example: --token "722c475bebb10...,XP,50"

Error: invalid --token quantity "abc": must be a non-negative integer

Error: duplicate token: policy_id "722c47..." + asset_name "XP" specified twice
```

## Acceptance Criteria

- [x] `project task create` accepts repeatable `--token "policy_id,asset_name,quantity"` flag
- [x] `project task update` accepts repeatable `--token` flag with `Changed()` guard
- [x] Tokens appear in API payload as `"tokens": [{policy_id, asset_name, quantity}]`
- [x] Omitting `--token` on update does not clear existing tokens
- [x] Invalid format (wrong field count) returns clear error with example
- [x] Invalid quantity (non-integer) returns clear error
- [x] Duplicate policy_id + asset_name rejected with error
- [x] Empty asset_name allowed (`"policy_id,,quantity"`)
- [x] Whitespace trimmed from all fields
- [x] Works with `--output json` (tokens in structured output)
- [x] No TTY required — fully composable
- [x] `--help` shows format description and example

## Composability Example

```bash
# Create task with tokens, capture result
RESULT=$(andamio project task create "$PROJECT_ID" \
  --title "Build dApp" \
  --lovelace 5000000 \
  --expiration 2026-06-01 \
  --token "$POLICY_ID,$ASSET_NAME,$QTY" \
  --output json)

echo "$RESULT" | jq '.data'
```

## Out of Scope

- `--clear-tokens` flag for removing all tokens on update (follow-up)
- Alternative token format (`policy_id.asset_name` Blockfrost-style concatenation)
- Updating task export to include tokens in YAML frontmatter (verify if already present; if not, separate follow-up)

## Sources

- **Origin brainstorm:** [docs/brainstorms/2026-03-18-project-task-management-brainstorm.md](../brainstorms/2026-03-18-project-task-management-brainstorm.md) — tokens included from start as YAML arrays, format: `[{policy_id, asset_name, quantity}]`
- **GitHub issue:** [#22 — Add --token flag to project task create](https://github.com/Andamio-Platform/andamio-cli/issues/22)
- **TaskToken struct:** `cmd/andamio/project_task_import.go:53-57`
- **StringArray flag pattern:** `cmd/andamio/tx_submit.go:36`
- **Token payload in import:** `cmd/andamio/project_task_import.go:249-251`
- **Composability audit:** `docs/solutions/architecture/cli-composability-audit-and-fix.md`
