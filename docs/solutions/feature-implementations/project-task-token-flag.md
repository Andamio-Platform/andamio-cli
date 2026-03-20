---
title: "Add --token flag to project task create/update for inline native asset assignment"
date: 2026-03-20
problem_type: feature-implementation
severity: medium
component: cmd/andamio/project_task.go
module: project-task
tags:
  - cli-flags
  - cobra
  - cardano-native-assets
  - token-assignment
  - string-array-flag
  - project-management
---

# Add --token flag to project task create/update

## Problem Statement

The `andamio project task create` and `project task update` commands had no way to assign Cardano native assets (such as XP tokens) to tasks inline from the command line. Users who needed to attach tokens to individual tasks were forced to go through the bulk Markdown import workflow, which was unnecessarily heavyweight for single-task operations. The API and task import already supported tokens — the CLI create/update commands just lacked the flag.

## Root Cause

The `TaskToken` struct and API payload support existed in `project_task_import.go` (for YAML frontmatter parsing), but `project_task.go` had no corresponding CLI flag to pass token data during inline task creation or updates.

## Solution

Five changes to `cmd/andamio/project_task.go`:

### 1. Flag registration in `init()`

```go
projectTaskCreateCmd.Flags().StringArray("token", nil,
    `Native asset token (repeatable, format: "policy_id,asset_name,quantity")`)
projectTaskUpdateCmd.Flags().StringArray("token", nil,
    `Native asset token (repeatable, format: "policy_id,asset_name,quantity")`)
```

`StringArray` is used instead of `StringSlice` because the value itself contains commas as the delimiter between fields. `StringSlice` would incorrectly split on those commas.

### 2. `parseTokenFlags` helper

Converts raw flag strings into `[]TaskToken` with validation:

```go
func parseTokenFlags(values []string) ([]TaskToken, error) {
    tokens := make([]TaskToken, 0, len(values))
    seen := make(map[string]bool)
    for _, v := range values {
        parts := strings.SplitN(v, ",", 3)
        if len(parts) != 3 {
            return nil, fmt.Errorf("invalid --token format %q: expected ...", v)
        }
        policyID := strings.TrimSpace(parts[0])
        assetName := strings.TrimSpace(parts[1])
        quantity := strings.TrimSpace(parts[2])
        // Validate: policy ID (56 hex chars), quantity (non-negative int), duplicates
        // Empty asset_name is allowed (Cardano policy-only tokens)
        ...
    }
    return tokens, nil
}
```

### 3. Create payload wiring

```go
tokenStrs, _ := cmd.Flags().GetStringArray("token")
if len(tokenStrs) > 0 {
    tokens, err := parseTokenFlags(tokenStrs)
    if err != nil { return err }
    payload["tokens"] = tokens
}
```

### 4. Update payload wiring (with `Changed()` guard)

```go
if cmd.Flags().Changed("token") {
    tokenStrs, _ := cmd.Flags().GetStringArray("token")
    tokens, err := parseTokenFlags(tokenStrs)
    if err != nil { return err }
    payload["tokens"] = tokens
}
```

### 5. Struct reuse

The existing `TaskToken` struct from `project_task_import.go` is reused directly since both files are in the same package (`main`).

## Key Design Decisions

- **Comma as in-value delimiter** -- Safe because Cardano policy IDs are hex strings and asset names are hex-encoded; neither can contain commas.
- **`StringArray` over `StringSlice`** -- Cobra's `StringSlice` splits on commas internally, which would break the `policyID,assetName,quantity` format. `StringArray` treats each `--token` flag as a single opaque string.
- **Replace semantics on update** -- When `--token` is passed during update, the entire token array is replaced. Consistent with how the API handles other array fields (SLTs, lessons).
- **Empty asset name is valid** -- Cardano supports policy-only tokens (no asset name), so `"policyID,,quantity"` is accepted.
- **Duplicate detection** -- A `seen` map keyed on `policyID:assetName` prevents accidentally sending the same token twice.
- **Policy ID validation** -- Must be exactly 56 hex characters. Catches truncated/malformed IDs client-side with a clear error instead of a cryptic API error.

## Prevention Strategies

### Default to `StringArray` for multi-value flags

`StringSlice` splits on commas *within* a single flag value. Always use `StringArray` unless you specifically want comma-separated values in a single invocation.

### Validate domain-specific inputs at the CLI boundary

Cardano identifiers have well-known formats (56 hex chars for policy IDs, 64 hex chars for tx hashes). Validate these in `RunE` before any API call. Return clear errors with the expected format.

### Search stdlib before writing helpers

Before writing utility functions, check Go's standard library. `strings.Contains`, `encoding/hex.DecodeString`, and `slices.Contains` cover most needs. A custom `contains` function was caught during review -- always use `strings.Contains`.

### Guard optional update flags with `Changed()`

For any update command, omitting a flag must mean "leave unchanged," not "set to zero value":

```go
if cmd.Flags().Changed("token") {
    payload["tokens"] = tokens
}
```

## Checklist for Adding CLI Flags

1. **Flag type**: Multi-value with structured content? Use `StringArray`, not `StringSlice`.
2. **Validation**: Known format (hex, UUID, date)? Add validation in `RunE` before API calls.
3. **Update semantics**: Wrap in `cmd.Flags().Changed()` guard.
4. **Reuse check**: Search `cmd/andamio/*.go` for existing structs and `internal/` for helpers.
5. **Help text**: Include expected format and example in flag description.
6. **Wire-up**: Register in `init()`, include in payload construction.

## Testing Guidance

- **Parsing correctness**: Verify `StringArray` preserves comma-containing values as single elements.
- **Validation rejection**: Test invalid inputs produce clear errors before any HTTP call.
- **Update semantics**: Test flag-provided (value in payload) vs flag-omitted (key absent from payload).
- **Use stdlib in tests**: Use `strings.Contains` for error matching, not custom helpers.

## Related Documentation

- `docs/plans/2026-03-20-feat-add-token-flag-to-task-create-update-plan.md` -- Implementation plan
- `docs/brainstorms/2026-03-18-project-task-management-brainstorm.md` -- Origin brainstorm where tokens were first specified
- `docs/solutions/architecture/cli-composability-audit-and-fix.md` -- Composability contract (progress to stderr, data to stdout)
- `docs/solutions/architecture/non-interactive-cli-stdin-picker-removal.md` -- Non-interactive CLI requirements
- `docs/solutions/security-issues/cli-security-hardening-input-validation.md` -- Input validation patterns

## Related Issues

- [GitHub Issue #22](https://github.com/Andamio-Platform/andamio-cli/issues/22) -- Add --token flag to project task create
- [GitHub PR #23](https://github.com/Andamio-Platform/andamio-cli/pull/23) -- Implementation PR
