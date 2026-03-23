---
title: "fix: Add --slt-hash flag for chain-only modules"
type: fix
status: completed
date: 2026-03-23
origin: GitHub Issue #42
---

# Add --slt-hash Flag for Chain-Only Modules

## Problem

All student evidence commands (`commit-tx`, `submit`, `update`) fail for chain-only modules because `resolveSltHash()` matches on `course_module_code`, which doesn't exist for `chain_only` source modules.

```
module feedback not found in course 76bab08...742
```

The module exists on-chain with a valid `slt_hash` but no `course_module_code`.

## Proposed Solution

Add `--slt-hash` as an alternative to `--module-code` on all student commands that call `resolveSltHash`. When `--slt-hash` is provided directly, skip resolution entirely.

### Commands to update

| Command | Uses `resolveSltHash` | Needs `--slt-hash` |
|---------|----------------------|-------------------|
| `course student commit-tx` | Yes (line 423) | Yes |
| `course student submit` | Yes (line 285) | Yes |
| `course student update` | No (uses `course_module_code` directly) | No |
| `course student commitment` | No (uses `course_module_code` directly) | No |
| `course student create` | No (uses `course_module_code` directly) | No |
| `course student leave` | No (uses `course_module_code` directly) | No |
| `course student claim` | No (uses `course_module_code` directly) | No |

Only `commit-tx` and `submit` call `resolveSltHash`. The other commands send `course_module_code` directly to the API — they don't need the hash.

### Implementation

**File: `cmd/andamio/course_student.go`**

Add `--slt-hash` flag to `commit-tx` and `submit` commands. Make `--module-code` no longer required when `--slt-hash` is provided.

```go
// In init(), for commit-tx and submit:
cmd.Flags().String("slt-hash", "", "SLT hash (use instead of --module-code for chain-only modules)")

// Remove MarkFlagRequired("module-code") — validate manually instead
```

**Resolution logic** (shared helper or inline):

```go
func resolveSltHashFromFlags(cmd *cobra.Command, c *client.Client, courseID string) (string, string, error) {
    sltHash, _ := cmd.Flags().GetString("slt-hash")
    moduleCode, _ := cmd.Flags().GetString("module-code")

    if sltHash != "" && moduleCode != "" {
        return "", "", fmt.Errorf("--slt-hash and --module-code are mutually exclusive")
    }
    if sltHash == "" && moduleCode == "" {
        return "", "", fmt.Errorf("either --module-code or --slt-hash is required")
    }

    if sltHash != "" {
        return sltHash, "", nil  // hash provided directly, no module code
    }

    // Resolve from module code
    hash, err := resolveSltHash(c, courseID, moduleCode)
    return hash, moduleCode, err
}
```

Returns `(sltHash, moduleCode, error)`. The `moduleCode` is needed for the off-chain evidence payload and error messages. When `--slt-hash` is used, `moduleCode` is empty — the off-chain payload uses `slt_hash` instead.

**Adjust off-chain payloads**: The `submit` command sends `slt_hash` (already has it). The `commit-tx` off-chain call also sends `slt_hash`. The `moduleCode` in metadata is optional. No payload changes needed.

## Acceptance Criteria

- [x] `--slt-hash` flag on `commit-tx` and `submit` commands
- [x] `--module-code` and `--slt-hash` are mutually exclusive
- [x] At least one of the two is required (clear error if both omitted)
- [x] Chain-only modules work: `commit-tx --slt-hash <hash> --course-id <id> --skey <key>`
- [x] Existing `--module-code` flow unchanged
- [x] Help text explains when to use `--slt-hash`
- [x] Progress messages show slt_hash when module-code is not available

## Sources

- GitHub Issue #42
- `cmd/andamio/helpers.go:283` — `resolveSltHash()`
- `cmd/andamio/course_student.go:423` — `commit-tx` uses `resolveSltHash`
- `cmd/andamio/course_student.go:285` — `submit` uses `resolveSltHash`
